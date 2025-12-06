package features

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"k8s.io/component-base/featuregate"
)

const (
	// BackupSidecar is a feature flag for the BackupSidecar feature
	BackupSidecar featuregate.Feature = "BackupSidecar"
)

// NewGate returns a new FeatureGate.
func NewGate() featuregate.MutableFeatureGate {
	gate := featuregate.NewFeatureGate()

	if err := gate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		BackupSidecar: {Default: false, PreRelease: featuregate.Alpha},
	}); err != nil {
		panic(err)
	}
	return gate
}

type contextKey struct{}

// Enabled indicates if a Feature is enabled in the Gate contained in ctx. It
// returns false when there is no Gate.
func Enabled(ctx context.Context, f featuregate.Feature) bool {
	gate, ok := ctx.Value(contextKey{}).(featuregate.FeatureGate)
	return ok && gate.Enabled(f)
}

// NewContextWithGate returns a copy of ctx containing gate. Check it using [Enabled].
func NewContextWithGate(ctx context.Context, gate featuregate.FeatureGate) context.Context {
	return context.WithValue(ctx, contextKey{}, gate)
}

// ShowEnabled returns all the features enabled in the Gate contained in ctx.
func ShowEnabled(ctx context.Context) string {
	featuresEnabled := []string{}
	if gate, ok := ctx.Value(contextKey{}).(interface {
		featuregate.FeatureGate
		GetAll() map[featuregate.Feature]featuregate.FeatureSpec
	}); ok {
		specs := gate.GetAll()
		for feature := range specs {
			// `gate.Enabled` first checks if the feature is enabled;
			// then (if not explicitly set by the user),
			// it checks if the feature is on/true by default
			if gate.Enabled(feature) {
				featuresEnabled = append(featuresEnabled, fmt.Sprintf("%s=true", feature))
			}
		}
	}
	slices.Sort(featuresEnabled)
	return strings.Join(featuresEnabled, ",")
}

// ShowAssigned returns the features enabled or disabled by Set and SetFromMap
// in the Gate contained in ctx.
func ShowAssigned(ctx context.Context) string {
	featuresAssigned := ""
	if gate, ok := ctx.Value(contextKey{}).(interface {
		featuregate.FeatureGate
		String() string
	}); ok {
		featuresAssigned = gate.String()
	}
	return featuresAssigned
}
