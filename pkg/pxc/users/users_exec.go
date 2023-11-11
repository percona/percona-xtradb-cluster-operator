package users

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/gocarina/gocsv"
	corev1 "k8s.io/api/core/v1"

	"github.com/percona/percona-xtradb-cluster-operator/clientcmd"
)

var sensitiveRegexp = regexp.MustCompile(":.*@")

type ManagerExec struct {
	client *clientcmd.Client
	pod    *corev1.Pod
	user   string
	pass   string
	host   string
}

func NewManagerExec(pod *corev1.Pod, cliCmd *clientcmd.Client, user, pass, host string) *ManagerExec {
	return &ManagerExec{client: cliCmd, pod: pod, user: user, pass: pass, host: host}
}

func (d *ManagerExec) exec(ctx context.Context, stm string, stdout, stderr *bytes.Buffer) error {
	cmd := []string{"mysql", "--database", "performance_schema", fmt.Sprintf("-p%s", d.pass), "-u", string(d.user), "-h", d.host, "-e", stm}

	err := d.client.Exec(d.pod, "pxc", cmd, nil, stdout, stderr, false)
	if err != nil {
		sout := sensitiveRegexp.ReplaceAllString(stdout.String(), ":*****@")
		serr := sensitiveRegexp.ReplaceAllString(stderr.String(), ":*****@")
		return errors.Wrapf(err, "run %s, stdout: %s, stderr: %s", cmd, sout, serr)
	}

	if strings.Contains(stderr.String(), "ERROR") {
		return fmt.Errorf("sql error: %s", stderr)
	}

	return nil
}

func (d *ManagerExec) query(ctx context.Context, query string, out interface{}) error {
	var errb, outb bytes.Buffer
	err := d.exec(ctx, query, &outb, &errb)
	if err != nil {
		return err
	}

	if !strings.Contains(errb.String(), "ERROR") && outb.Len() == 0 {
		return sql.ErrNoRows
	}

	csv := csv.NewReader(bytes.NewReader(outb.Bytes()))
	csv.Comma = '\t'

	if err = gocsv.UnmarshalCSV(csv, out); err != nil {
		return err
	}

	return nil
}

func (u *ManagerExec) CreateOperatorUser(ctx context.Context, pass string) error {
	var errb, outb bytes.Buffer

	q := fmt.Sprintf("CREATE USER IF NOT EXISTS 'operator'@'%%' IDENTIFIED BY '%s'", pass)
	err := u.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "create operator user")
	}

	outb.Reset()
	errb.Reset()
	err = u.exec(ctx, "GRANT ALL ON *.* TO 'operator'@'%' WITH GRANT OPTION", &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "grant operator user")
	}

	return nil
}

// UpdateUserPassWithoutDP updates user pass without Dual Password
// feature introduced in MsSQL 8
func (u *ManagerExec) UpdateUserPassWithoutDP(ctx context.Context, user *SysUser) error {
	if user == nil {
		return nil
	}

	var errb, outb bytes.Buffer
	for _, host := range user.Hosts {
		q := fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s", user.Name, host, user.Pass)
		err := u.exec(ctx, q, &outb, &errb)
		if err != nil {
			return errors.Wrap(err, "update password")
		}
	}

	return nil
}

// UpdateUserPass updates user passwords but retains the current password
// using Dual Password feature of MySQL 8.
func (m *ManagerExec) UpdateUserPass(ctx context.Context, user *SysUser) error {
	if user == nil {
		return nil
	}

	for _, host := range user.Hosts {
		var errb, outb bytes.Buffer
		q := fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s' RETAIN CURRENT PASSWORD", user.Name, host, user.Pass)
		err := m.exec(ctx, q, &outb, &errb)
		if err != nil {
			return err
		}
	}

	return nil
}

// DiscardOldPassword discards old passwords of given users
func (m *ManagerExec) DiscardOldPassword(ctx context.Context, user *SysUser) error {
	if user == nil {
		return nil
	}

	for _, host := range user.Hosts {
		var errb, outb bytes.Buffer
		q := fmt.Sprintf("ALTER USER '%s'@'%s' DISCARD OLD PASSWORD", user.Name, host)
		err := m.exec(ctx, q, &outb, &errb)
		if err != nil {
			return err
		}
	}

	return nil
}

// IsOldPassDiscarded checks if old password is discarded
func (m *ManagerExec) IsOldPassDiscarded(ctx context.Context, user *SysUser) (bool, error) {
	rows := []*struct {
		Attr string `csv:"attr"`
	}{}

	err := m.query(ctx, fmt.Sprintf("SELECT User_attributes as attr FROM mysql.user WHERE user='%s'", user.Name), &rows)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, errors.Wrap(err, "select User_attributes field")
	}

	if len(rows[0].Attr) > 0 {
		return false, nil
	}

	return true, nil
}

func (u *ManagerExec) UpdateProxyUser(ctx context.Context, user *SysUser) error {
	switch user.Name {
	case ProxyAdmin:
		q := fmt.Sprintf(`
			UPDATE global_variables SET variable_value='%s' WHERE variable_name='admin-admin_credentials';
			UPDATE global_variables SET variable_value='%s' WHERE variable_name='admin-cluster_password';
			LOAD ADMIN VARIABLES TO RUNTIME;
			SAVE ADMIN VARIABLES TO DISK;	
		`, "proxyadmin:"+user.Pass, user.Pass)

		var errb, outb bytes.Buffer
		err := u.exec(ctx, q, &outb, &errb)
		if err != nil {
			return errors.Wrap(err, "update proxy admin password")
		}
	case Monitor:
		q := fmt.Sprintf(`
			UPDATE global_variables SET variable_value='%s' WHERE variable_name='mysql-monitor_password';
			LOAD MYSQL VARIABLES TO RUNTIME;
			SAVE MYSQL VARIABLES TO DISK;
		`, user.Pass)

		var errb, outb bytes.Buffer
		err := u.exec(ctx, q, &outb, &errb)
		if err != nil {
			return errors.Wrap(err, "update proxy monitor password")
		}
	}

	return nil
}

// Update160MonitorUserGrant grants SERVICE_CONNECTION_ADMIN rights to the monitor user
// if pxc version is 8 or more and sets the MAX_USER_CONNECTIONS parameter to 100 (empirically determined)
func (u *ManagerExec) Update160MonitorUserGrant(ctx context.Context, pass string) (err error) {
	q := fmt.Sprintf(`
		CREATE USER IF NOT EXISTS 'monitor'@'%%' IDENTIFIED BY '%s';
		GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'%%';
		ALTER USER 'monitor'@'%%' WITH MAX_USER_CONNECTIONS 100;
	`, pass)

	var errb, outb bytes.Buffer
	err = u.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "update monitor user grants")
	}

	return nil
}

// Update170XtrabackupUser grants all needed rights to the xtrabackup user
func (u *ManagerExec) Update170XtrabackupUser(ctx context.Context, pass string) (err error) {
	q := fmt.Sprintf(`
		CREATE USER IF NOT EXISTS 'xtrabackup'@'%%' IDENTIFIED BY '%s';
		GRANT ALL ON *.* TO 'xtrabackup'@'%%';
	`, pass)
	var errb, outb bytes.Buffer
	err = u.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "update xtrabackup user grants")
	}

	return nil
}

// Update1100SystemUserPrivilege grants system_user privilege for monitor and clustercheck users
func (u *ManagerExec) Update1100SystemUserPrivilege(ctx context.Context, user *SysUser) (err error) {
	switch user.Name {
	case Monitor:
		var errb, outb bytes.Buffer
		if err := u.exec(ctx, "GRANT SYSTEM_USER ON *.* TO 'monitor'@'%'", &outb, &errb); err != nil {
			return errors.Wrap(err, "monitor user")
		}
	case Clustercheck:
		var errb, outb bytes.Buffer
		if err := u.exec(ctx, "GRANT SYSTEM_USER ON *.* TO 'clustercheck'@'localhost'", &outb, &errb); err != nil {
			return errors.Wrap(err, "clustercheck user")
		}
	}

	return nil
}

func (u *ManagerExec) CreateReplicationUser(ctx context.Context, password string) error {
	q := fmt.Sprintf(`
		CREATE USER IF NOT EXISTS 'replication'@'%%' IDENTIFIED BY '%s';
		GRANT REPLICATION SLAVE ON *.* to 'replication'@'%%';
	`, password)
	var errb, outb bytes.Buffer
	err := u.exec(ctx, q, &outb, &errb)
	if err != nil {
		return errors.Wrap(err, "create replication user")
	}

	return nil
}

// UpdatePassExpirationPolicy sets user password expiration policy to never
func (u *ManagerExec) UpdatePassExpirationPolicy(ctx context.Context, user *SysUser) error {
	if user == nil {
		return nil
	}

	for _, host := range user.Hosts {
		var errb, outb bytes.Buffer
		q := fmt.Sprintf("ALTER USER '%s'@'%s' PASSWORD EXPIRE NEVER", user.Name, host)
		err := u.exec(ctx, q, &outb, &errb)
		if err != nil {
			return err
		}
	}
	return nil
}
