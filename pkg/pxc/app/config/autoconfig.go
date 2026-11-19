package config

import (
	"regexp"
	"strconv"

	mysqlcalc "github.com/Tusamarco/mysqloperatorcalculator/v2/calculator"
	v "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

var (
	ErrInsufficientStorage = errors.New("data volume is too small for the calculated configuration")
	ErrVersionUnsupported  = errors.New("mysql version is out of the autoconfig supported range")
)

const maxPreallocatedPercent = 25

var imageVersion = regexp.MustCompile(`\d+\.\d+\.\d+`)

type AutoConfigRequest struct {
	Version  string
	CPU      *resource.Quantity
	Memory   *resource.Quantity
	Storage  int64
	LoadType api.AutoConfigLoadType
	User     MySQLdConfig
}

// AutoConfigParams calculates the mysqld options for a PXC node.
func AutoConfigParams(req AutoConfigRequest) (MySQLdConfig, error) {
	if req.CPU == nil || req.CPU.Sign() <= 0 {
		return nil, errors.New("cpu is required for autoconfig")
	}
	if req.Memory == nil || req.Memory.Sign() <= 0 {
		return nil, errors.New("memory is required for autoconfig")
	}

	ver, err := parseVersion(req.Version)
	if err != nil {
		return nil, err
	}

	calcReq := mysqlcalc.ConfigurationRequest{
		DBType:         mysqlcalc.DBTypePXC,
		LoadType:       mysqlcalc.LoadType{ID: calculatorLoadType(req.LoadType)},
		Output:         mysqlcalc.ResultOutputFormatJSON,
		MySQLVersion:   ver,
		MySQLDedicated: true,
		Dimension: mysqlcalc.Dimension{
			ID:          mysqlcalc.DimensionOpen,
			Cpu:         int(req.CPU.MilliValue()),
			MemoryBytes: float64(req.Memory.Value()),
		},
	}

	var conf mysqlcalc.Configuration
	conf.Init()

	calc := new(mysqlcalc.MySQLOperatorCalculator)
	calc.Init(calcReq, conf)
	calcErr, msg, families := calc.GetCalculate()
	if calcErr != nil {
		return nil, errors.Wrap(calcErr, "calculate mysql configuration")
	}
	if msg.Type == mysqlcalc.ErrorexecI {
		return nil, errors.Errorf("calculate mysql configuration: %s: %s", msg.Name, msg.Text)
	}
	family, ok := families[mysqlcalc.FamilyTypeMySQL]
	if !ok || len(family.Groups) == 0 {
		return nil, errors.Errorf("calculate mysql configuration: no mysql parameters returned: %s", msg.Text)
	}

	params := make(MySQLdConfig)
	for groupName, group := range family.Groups {
		switch groupName {
		case mysqlcalc.GroupNameResources, mysqlcalc.GroupNameProbes, "readinessProbe", "livenessProbe":
			continue
		}
		for _, p := range group.Parameters {
			name := NormalizeOptionName(p.Name)
			if req.User.HasVariable(name) {
				continue
			}
			params[name] = p.Value
		}
	}

	if err := checkPreallocatedFits(req.Storage, params); err != nil {
		return nil, err
	}

	return params, nil
}

// AutoConfigVersion returns the server version to calculate for: the one in
// the image tag, so an upgrade is tuned for its target, else the one the
// operator recorded.
func AutoConfigVersion(cr *api.PerconaXtraDBCluster) string {
	if cr.Spec.PXC != nil {
		if v := imageVersion.FindString(cr.Spec.PXC.Image); v != "" {
			return v
		}
	}
	return cr.Status.PXC.Version
}

// EffectiveResource returns the limit, else the request, else nil.
func EffectiveResource(res corev1.ResourceRequirements, name corev1.ResourceName) *resource.Quantity {
	if q, ok := res.Limits[name]; ok {
		return &q
	}
	if q, ok := res.Requests[name]; ok {
		return &q
	}
	return nil
}

func parseVersion(s string) (mysqlcalc.Version, error) {
	parsed, err := v.NewVersion(s)
	if err != nil {
		return mysqlcalc.Version{}, errors.Wrapf(err, "parse mysql version %q", s)
	}

	seg := parsed.Segments()
	ver := mysqlcalc.Version{Major: seg[0], Minor: seg[1], Patch: seg[2]}
	if compareVersions(ver, mysqlcalc.MySQLMinSupported) < 0 || compareVersions(ver, mysqlcalc.MySQLMaxSupported) > 0 {
		return mysqlcalc.Version{}, errors.Wrapf(ErrVersionUnsupported, "%s is outside %s..%s",
			s, versionString(mysqlcalc.MySQLMinSupported), versionString(mysqlcalc.MySQLMaxSupported))
	}
	return ver, nil
}

func compareVersions(a, b mysqlcalc.Version) int {
	if a.Major != b.Major {
		return a.Major - b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor - b.Minor
	}
	return a.Patch - b.Patch
}

func versionString(ver mysqlcalc.Version) string {
	return strconv.Itoa(ver.Major) + "." + strconv.Itoa(ver.Minor) + "." + strconv.Itoa(ver.Patch)
}

func calculatorLoadType(lt api.AutoConfigLoadType) int {
	switch lt {
	case api.AutoConfigLoadTypeMostlyReads:
		return mysqlcalc.LoadTypeMostlyReads
	case api.AutoConfigLoadTypeEqualReadsWrites:
		return mysqlcalc.LoadTypeEqualReadsWrites
	case api.AutoConfigLoadTypeHeavyWrites:
		return mysqlcalc.LoadTypeHeavyWrites
	default:
		return mysqlcalc.LoadTypeSomeWrites
	}
}

func checkPreallocatedFits(storage int64, params MySQLdConfig) error {
	if storage == 0 {
		return nil
	}

	redo, err := redoLogBytes(params)
	if err != nil {
		return err
	}
	gcache, err := intOption(parseProviderOptions(params[ProviderOptionsVariable]), "gcache.size")
	if err != nil {
		return err
	}

	needed := redo + gcache
	if needed <= storage*maxPreallocatedPercent/100 {
		return nil
	}

	return errors.Wrapf(ErrInsufficientStorage,
		"the calculated redo log and gcache need %d bytes, more than %d%% of the %d byte data volume; "+
			"give pxc.volumeSpec.persistentVolumeClaim at least %d bytes, or lower the memory in pxc.resources",
		needed, maxPreallocatedPercent, storage, needed*100/maxPreallocatedPercent)
}

func redoLogBytes(params MySQLdConfig) (int64, error) {
	capacity, err := intOption(params, "innodb_redo_log_capacity")
	if err != nil || capacity > 0 {
		return capacity, err
	}

	size, err := intOption(params, "innodb_log_file_size")
	if err != nil {
		return 0, err
	}
	files, err := intOption(params, "innodb_log_files_in_group")
	if err != nil {
		return 0, err
	}
	return size * max(files, 1), nil
}

func intOption(params map[string]string, name string) (int64, error) {
	value, ok := params[name]
	if !ok {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parse %s value %q", name, value)
	}
	return n, nil
}
