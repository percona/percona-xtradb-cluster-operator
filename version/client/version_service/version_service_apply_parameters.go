// Code generated by go-swagger; DO NOT EDIT.

package version_service

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// NewVersionServiceApplyParams creates a new VersionServiceApplyParams object,
// with the default timeout for this client.
//
// Default values are not hydrated, since defaults are normally applied by the API server side.
//
// To enforce default values in parameter, use SetDefaults or WithDefaults.
func NewVersionServiceApplyParams() *VersionServiceApplyParams {
	return &VersionServiceApplyParams{
		timeout: cr.DefaultTimeout,
	}
}

// NewVersionServiceApplyParamsWithTimeout creates a new VersionServiceApplyParams object
// with the ability to set a timeout on a request.
func NewVersionServiceApplyParamsWithTimeout(timeout time.Duration) *VersionServiceApplyParams {
	return &VersionServiceApplyParams{
		timeout: timeout,
	}
}

// NewVersionServiceApplyParamsWithContext creates a new VersionServiceApplyParams object
// with the ability to set a context for a request.
func NewVersionServiceApplyParamsWithContext(ctx context.Context) *VersionServiceApplyParams {
	return &VersionServiceApplyParams{
		Context: ctx,
	}
}

// NewVersionServiceApplyParamsWithHTTPClient creates a new VersionServiceApplyParams object
// with the ability to set a custom HTTPClient for a request.
func NewVersionServiceApplyParamsWithHTTPClient(client *http.Client) *VersionServiceApplyParams {
	return &VersionServiceApplyParams{
		HTTPClient: client,
	}
}

/*
VersionServiceApplyParams contains all the parameters to send to the API endpoint

	for the version service apply operation.

	Typically these are written to a http.Request.
*/
type VersionServiceApplyParams struct {

	// Apply.
	Apply string

	// BackupVersion.
	BackupVersion *string

	// BackupsEnabled.
	BackupsEnabled *bool

	// ClusterSize.
	//
	// Format: int32
	ClusterSize *int32

	// ClusterWideEnabled.
	ClusterWideEnabled *bool

	// CustomResourceUID.
	CustomResourceUID *string

	// DatabaseVersion.
	DatabaseVersion *string

	// Extensions.
	Extensions *string

	// HaproxyVersion.
	HaproxyVersion *string

	// HashicorpVaultEnabled.
	HashicorpVaultEnabled *bool

	// HelmDeployCr.
	HelmDeployCr *bool

	// HelmDeployOperator.
	HelmDeployOperator *bool

	// KubeVersion.
	KubeVersion *string

	// LogCollectorVersion.
	LogCollectorVersion *string

	// NamespaceUID.
	NamespaceUID *string

	// OperatorVersion.
	OperatorVersion string

	// PhysicalBackupScheduled.
	PhysicalBackupScheduled *bool

	// PitrEnabled.
	PitrEnabled *bool

	// Platform.
	Platform *string

	// PmmEnabled.
	PmmEnabled *bool

	// PmmVersion.
	PmmVersion *string

	// Product.
	Product string

	// ProxysqlVersion.
	ProxysqlVersion *string

	// ShardingEnabled.
	ShardingEnabled *bool

	// SidecarsUsed.
	SidecarsUsed *bool

	// UserManagementEnabled.
	UserManagementEnabled *bool

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithDefaults hydrates default values in the version service apply params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VersionServiceApplyParams) WithDefaults() *VersionServiceApplyParams {
	o.SetDefaults()
	return o
}

// SetDefaults hydrates default values in the version service apply params (not the query body).
//
// All values with no default are reset to their zero value.
func (o *VersionServiceApplyParams) SetDefaults() {
	// no default values defined for this parameter
}

// WithTimeout adds the timeout to the version service apply params
func (o *VersionServiceApplyParams) WithTimeout(timeout time.Duration) *VersionServiceApplyParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the version service apply params
func (o *VersionServiceApplyParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the version service apply params
func (o *VersionServiceApplyParams) WithContext(ctx context.Context) *VersionServiceApplyParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the version service apply params
func (o *VersionServiceApplyParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the version service apply params
func (o *VersionServiceApplyParams) WithHTTPClient(client *http.Client) *VersionServiceApplyParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the version service apply params
func (o *VersionServiceApplyParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithApply adds the apply to the version service apply params
func (o *VersionServiceApplyParams) WithApply(apply string) *VersionServiceApplyParams {
	o.SetApply(apply)
	return o
}

// SetApply adds the apply to the version service apply params
func (o *VersionServiceApplyParams) SetApply(apply string) {
	o.Apply = apply
}

// WithBackupVersion adds the backupVersion to the version service apply params
func (o *VersionServiceApplyParams) WithBackupVersion(backupVersion *string) *VersionServiceApplyParams {
	o.SetBackupVersion(backupVersion)
	return o
}

// SetBackupVersion adds the backupVersion to the version service apply params
func (o *VersionServiceApplyParams) SetBackupVersion(backupVersion *string) {
	o.BackupVersion = backupVersion
}

// WithBackupsEnabled adds the backupsEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithBackupsEnabled(backupsEnabled *bool) *VersionServiceApplyParams {
	o.SetBackupsEnabled(backupsEnabled)
	return o
}

// SetBackupsEnabled adds the backupsEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetBackupsEnabled(backupsEnabled *bool) {
	o.BackupsEnabled = backupsEnabled
}

// WithClusterSize adds the clusterSize to the version service apply params
func (o *VersionServiceApplyParams) WithClusterSize(clusterSize *int32) *VersionServiceApplyParams {
	o.SetClusterSize(clusterSize)
	return o
}

// SetClusterSize adds the clusterSize to the version service apply params
func (o *VersionServiceApplyParams) SetClusterSize(clusterSize *int32) {
	o.ClusterSize = clusterSize
}

// WithClusterWideEnabled adds the clusterWideEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithClusterWideEnabled(clusterWideEnabled *bool) *VersionServiceApplyParams {
	o.SetClusterWideEnabled(clusterWideEnabled)
	return o
}

// SetClusterWideEnabled adds the clusterWideEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetClusterWideEnabled(clusterWideEnabled *bool) {
	o.ClusterWideEnabled = clusterWideEnabled
}

// WithCustomResourceUID adds the customResourceUID to the version service apply params
func (o *VersionServiceApplyParams) WithCustomResourceUID(customResourceUID *string) *VersionServiceApplyParams {
	o.SetCustomResourceUID(customResourceUID)
	return o
}

// SetCustomResourceUID adds the customResourceUid to the version service apply params
func (o *VersionServiceApplyParams) SetCustomResourceUID(customResourceUID *string) {
	o.CustomResourceUID = customResourceUID
}

// WithDatabaseVersion adds the databaseVersion to the version service apply params
func (o *VersionServiceApplyParams) WithDatabaseVersion(databaseVersion *string) *VersionServiceApplyParams {
	o.SetDatabaseVersion(databaseVersion)
	return o
}

// SetDatabaseVersion adds the databaseVersion to the version service apply params
func (o *VersionServiceApplyParams) SetDatabaseVersion(databaseVersion *string) {
	o.DatabaseVersion = databaseVersion
}

// WithExtensions adds the extensions to the version service apply params
func (o *VersionServiceApplyParams) WithExtensions(extensions *string) *VersionServiceApplyParams {
	o.SetExtensions(extensions)
	return o
}

// SetExtensions adds the extensions to the version service apply params
func (o *VersionServiceApplyParams) SetExtensions(extensions *string) {
	o.Extensions = extensions
}

// WithHaproxyVersion adds the haproxyVersion to the version service apply params
func (o *VersionServiceApplyParams) WithHaproxyVersion(haproxyVersion *string) *VersionServiceApplyParams {
	o.SetHaproxyVersion(haproxyVersion)
	return o
}

// SetHaproxyVersion adds the haproxyVersion to the version service apply params
func (o *VersionServiceApplyParams) SetHaproxyVersion(haproxyVersion *string) {
	o.HaproxyVersion = haproxyVersion
}

// WithHashicorpVaultEnabled adds the hashicorpVaultEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithHashicorpVaultEnabled(hashicorpVaultEnabled *bool) *VersionServiceApplyParams {
	o.SetHashicorpVaultEnabled(hashicorpVaultEnabled)
	return o
}

// SetHashicorpVaultEnabled adds the hashicorpVaultEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetHashicorpVaultEnabled(hashicorpVaultEnabled *bool) {
	o.HashicorpVaultEnabled = hashicorpVaultEnabled
}

// WithHelmDeployCr adds the helmDeployCr to the version service apply params
func (o *VersionServiceApplyParams) WithHelmDeployCr(helmDeployCr *bool) *VersionServiceApplyParams {
	o.SetHelmDeployCr(helmDeployCr)
	return o
}

// SetHelmDeployCr adds the helmDeployCr to the version service apply params
func (o *VersionServiceApplyParams) SetHelmDeployCr(helmDeployCr *bool) {
	o.HelmDeployCr = helmDeployCr
}

// WithHelmDeployOperator adds the helmDeployOperator to the version service apply params
func (o *VersionServiceApplyParams) WithHelmDeployOperator(helmDeployOperator *bool) *VersionServiceApplyParams {
	o.SetHelmDeployOperator(helmDeployOperator)
	return o
}

// SetHelmDeployOperator adds the helmDeployOperator to the version service apply params
func (o *VersionServiceApplyParams) SetHelmDeployOperator(helmDeployOperator *bool) {
	o.HelmDeployOperator = helmDeployOperator
}

// WithKubeVersion adds the kubeVersion to the version service apply params
func (o *VersionServiceApplyParams) WithKubeVersion(kubeVersion *string) *VersionServiceApplyParams {
	o.SetKubeVersion(kubeVersion)
	return o
}

// SetKubeVersion adds the kubeVersion to the version service apply params
func (o *VersionServiceApplyParams) SetKubeVersion(kubeVersion *string) {
	o.KubeVersion = kubeVersion
}

// WithLogCollectorVersion adds the logCollectorVersion to the version service apply params
func (o *VersionServiceApplyParams) WithLogCollectorVersion(logCollectorVersion *string) *VersionServiceApplyParams {
	o.SetLogCollectorVersion(logCollectorVersion)
	return o
}

// SetLogCollectorVersion adds the logCollectorVersion to the version service apply params
func (o *VersionServiceApplyParams) SetLogCollectorVersion(logCollectorVersion *string) {
	o.LogCollectorVersion = logCollectorVersion
}

// WithNamespaceUID adds the namespaceUID to the version service apply params
func (o *VersionServiceApplyParams) WithNamespaceUID(namespaceUID *string) *VersionServiceApplyParams {
	o.SetNamespaceUID(namespaceUID)
	return o
}

// SetNamespaceUID adds the namespaceUid to the version service apply params
func (o *VersionServiceApplyParams) SetNamespaceUID(namespaceUID *string) {
	o.NamespaceUID = namespaceUID
}

// WithOperatorVersion adds the operatorVersion to the version service apply params
func (o *VersionServiceApplyParams) WithOperatorVersion(operatorVersion string) *VersionServiceApplyParams {
	o.SetOperatorVersion(operatorVersion)
	return o
}

// SetOperatorVersion adds the operatorVersion to the version service apply params
func (o *VersionServiceApplyParams) SetOperatorVersion(operatorVersion string) {
	o.OperatorVersion = operatorVersion
}

// WithPhysicalBackupScheduled adds the physicalBackupScheduled to the version service apply params
func (o *VersionServiceApplyParams) WithPhysicalBackupScheduled(physicalBackupScheduled *bool) *VersionServiceApplyParams {
	o.SetPhysicalBackupScheduled(physicalBackupScheduled)
	return o
}

// SetPhysicalBackupScheduled adds the physicalBackupScheduled to the version service apply params
func (o *VersionServiceApplyParams) SetPhysicalBackupScheduled(physicalBackupScheduled *bool) {
	o.PhysicalBackupScheduled = physicalBackupScheduled
}

// WithPitrEnabled adds the pitrEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithPitrEnabled(pitrEnabled *bool) *VersionServiceApplyParams {
	o.SetPitrEnabled(pitrEnabled)
	return o
}

// SetPitrEnabled adds the pitrEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetPitrEnabled(pitrEnabled *bool) {
	o.PitrEnabled = pitrEnabled
}

// WithPlatform adds the platform to the version service apply params
func (o *VersionServiceApplyParams) WithPlatform(platform *string) *VersionServiceApplyParams {
	o.SetPlatform(platform)
	return o
}

// SetPlatform adds the platform to the version service apply params
func (o *VersionServiceApplyParams) SetPlatform(platform *string) {
	o.Platform = platform
}

// WithPmmEnabled adds the pmmEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithPmmEnabled(pmmEnabled *bool) *VersionServiceApplyParams {
	o.SetPmmEnabled(pmmEnabled)
	return o
}

// SetPmmEnabled adds the pmmEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetPmmEnabled(pmmEnabled *bool) {
	o.PmmEnabled = pmmEnabled
}

// WithPmmVersion adds the pmmVersion to the version service apply params
func (o *VersionServiceApplyParams) WithPmmVersion(pmmVersion *string) *VersionServiceApplyParams {
	o.SetPmmVersion(pmmVersion)
	return o
}

// SetPmmVersion adds the pmmVersion to the version service apply params
func (o *VersionServiceApplyParams) SetPmmVersion(pmmVersion *string) {
	o.PmmVersion = pmmVersion
}

// WithProduct adds the product to the version service apply params
func (o *VersionServiceApplyParams) WithProduct(product string) *VersionServiceApplyParams {
	o.SetProduct(product)
	return o
}

// SetProduct adds the product to the version service apply params
func (o *VersionServiceApplyParams) SetProduct(product string) {
	o.Product = product
}

// WithProxysqlVersion adds the proxysqlVersion to the version service apply params
func (o *VersionServiceApplyParams) WithProxysqlVersion(proxysqlVersion *string) *VersionServiceApplyParams {
	o.SetProxysqlVersion(proxysqlVersion)
	return o
}

// SetProxysqlVersion adds the proxysqlVersion to the version service apply params
func (o *VersionServiceApplyParams) SetProxysqlVersion(proxysqlVersion *string) {
	o.ProxysqlVersion = proxysqlVersion
}

// WithShardingEnabled adds the shardingEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithShardingEnabled(shardingEnabled *bool) *VersionServiceApplyParams {
	o.SetShardingEnabled(shardingEnabled)
	return o
}

// SetShardingEnabled adds the shardingEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetShardingEnabled(shardingEnabled *bool) {
	o.ShardingEnabled = shardingEnabled
}

// WithSidecarsUsed adds the sidecarsUsed to the version service apply params
func (o *VersionServiceApplyParams) WithSidecarsUsed(sidecarsUsed *bool) *VersionServiceApplyParams {
	o.SetSidecarsUsed(sidecarsUsed)
	return o
}

// SetSidecarsUsed adds the sidecarsUsed to the version service apply params
func (o *VersionServiceApplyParams) SetSidecarsUsed(sidecarsUsed *bool) {
	o.SidecarsUsed = sidecarsUsed
}

// WithUserManagementEnabled adds the userManagementEnabled to the version service apply params
func (o *VersionServiceApplyParams) WithUserManagementEnabled(userManagementEnabled *bool) *VersionServiceApplyParams {
	o.SetUserManagementEnabled(userManagementEnabled)
	return o
}

// SetUserManagementEnabled adds the userManagementEnabled to the version service apply params
func (o *VersionServiceApplyParams) SetUserManagementEnabled(userManagementEnabled *bool) {
	o.UserManagementEnabled = userManagementEnabled
}

// WriteToRequest writes these params to a swagger request
func (o *VersionServiceApplyParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param apply
	if err := r.SetPathParam("apply", o.Apply); err != nil {
		return err
	}

	if o.BackupVersion != nil {

		// query param backupVersion
		var qrBackupVersion string

		if o.BackupVersion != nil {
			qrBackupVersion = *o.BackupVersion
		}
		qBackupVersion := qrBackupVersion
		if qBackupVersion != "" {

			if err := r.SetQueryParam("backupVersion", qBackupVersion); err != nil {
				return err
			}
		}
	}

	if o.BackupsEnabled != nil {

		// query param backupsEnabled
		var qrBackupsEnabled bool

		if o.BackupsEnabled != nil {
			qrBackupsEnabled = *o.BackupsEnabled
		}
		qBackupsEnabled := swag.FormatBool(qrBackupsEnabled)
		if qBackupsEnabled != "" {

			if err := r.SetQueryParam("backupsEnabled", qBackupsEnabled); err != nil {
				return err
			}
		}
	}

	if o.ClusterSize != nil {

		// query param clusterSize
		var qrClusterSize int32

		if o.ClusterSize != nil {
			qrClusterSize = *o.ClusterSize
		}
		qClusterSize := swag.FormatInt32(qrClusterSize)
		if qClusterSize != "" {

			if err := r.SetQueryParam("clusterSize", qClusterSize); err != nil {
				return err
			}
		}
	}

	if o.ClusterWideEnabled != nil {

		// query param clusterWideEnabled
		var qrClusterWideEnabled bool

		if o.ClusterWideEnabled != nil {
			qrClusterWideEnabled = *o.ClusterWideEnabled
		}
		qClusterWideEnabled := swag.FormatBool(qrClusterWideEnabled)
		if qClusterWideEnabled != "" {

			if err := r.SetQueryParam("clusterWideEnabled", qClusterWideEnabled); err != nil {
				return err
			}
		}
	}

	if o.CustomResourceUID != nil {

		// query param customResourceUid
		var qrCustomResourceUID string

		if o.CustomResourceUID != nil {
			qrCustomResourceUID = *o.CustomResourceUID
		}
		qCustomResourceUID := qrCustomResourceUID
		if qCustomResourceUID != "" {

			if err := r.SetQueryParam("customResourceUid", qCustomResourceUID); err != nil {
				return err
			}
		}
	}

	if o.DatabaseVersion != nil {

		// query param databaseVersion
		var qrDatabaseVersion string

		if o.DatabaseVersion != nil {
			qrDatabaseVersion = *o.DatabaseVersion
		}
		qDatabaseVersion := qrDatabaseVersion
		if qDatabaseVersion != "" {

			if err := r.SetQueryParam("databaseVersion", qDatabaseVersion); err != nil {
				return err
			}
		}
	}

	if o.Extensions != nil {

		// query param extensions
		var qrExtensions string

		if o.Extensions != nil {
			qrExtensions = *o.Extensions
		}
		qExtensions := qrExtensions
		if qExtensions != "" {

			if err := r.SetQueryParam("extensions", qExtensions); err != nil {
				return err
			}
		}
	}

	if o.HaproxyVersion != nil {

		// query param haproxyVersion
		var qrHaproxyVersion string

		if o.HaproxyVersion != nil {
			qrHaproxyVersion = *o.HaproxyVersion
		}
		qHaproxyVersion := qrHaproxyVersion
		if qHaproxyVersion != "" {

			if err := r.SetQueryParam("haproxyVersion", qHaproxyVersion); err != nil {
				return err
			}
		}
	}

	if o.HashicorpVaultEnabled != nil {

		// query param hashicorpVaultEnabled
		var qrHashicorpVaultEnabled bool

		if o.HashicorpVaultEnabled != nil {
			qrHashicorpVaultEnabled = *o.HashicorpVaultEnabled
		}
		qHashicorpVaultEnabled := swag.FormatBool(qrHashicorpVaultEnabled)
		if qHashicorpVaultEnabled != "" {

			if err := r.SetQueryParam("hashicorpVaultEnabled", qHashicorpVaultEnabled); err != nil {
				return err
			}
		}
	}

	if o.HelmDeployCr != nil {

		// query param helmDeployCr
		var qrHelmDeployCr bool

		if o.HelmDeployCr != nil {
			qrHelmDeployCr = *o.HelmDeployCr
		}
		qHelmDeployCr := swag.FormatBool(qrHelmDeployCr)
		if qHelmDeployCr != "" {

			if err := r.SetQueryParam("helmDeployCr", qHelmDeployCr); err != nil {
				return err
			}
		}
	}

	if o.HelmDeployOperator != nil {

		// query param helmDeployOperator
		var qrHelmDeployOperator bool

		if o.HelmDeployOperator != nil {
			qrHelmDeployOperator = *o.HelmDeployOperator
		}
		qHelmDeployOperator := swag.FormatBool(qrHelmDeployOperator)
		if qHelmDeployOperator != "" {

			if err := r.SetQueryParam("helmDeployOperator", qHelmDeployOperator); err != nil {
				return err
			}
		}
	}

	if o.KubeVersion != nil {

		// query param kubeVersion
		var qrKubeVersion string

		if o.KubeVersion != nil {
			qrKubeVersion = *o.KubeVersion
		}
		qKubeVersion := qrKubeVersion
		if qKubeVersion != "" {

			if err := r.SetQueryParam("kubeVersion", qKubeVersion); err != nil {
				return err
			}
		}
	}

	if o.LogCollectorVersion != nil {

		// query param logCollectorVersion
		var qrLogCollectorVersion string

		if o.LogCollectorVersion != nil {
			qrLogCollectorVersion = *o.LogCollectorVersion
		}
		qLogCollectorVersion := qrLogCollectorVersion
		if qLogCollectorVersion != "" {

			if err := r.SetQueryParam("logCollectorVersion", qLogCollectorVersion); err != nil {
				return err
			}
		}
	}

	if o.NamespaceUID != nil {

		// query param namespaceUid
		var qrNamespaceUID string

		if o.NamespaceUID != nil {
			qrNamespaceUID = *o.NamespaceUID
		}
		qNamespaceUID := qrNamespaceUID
		if qNamespaceUID != "" {

			if err := r.SetQueryParam("namespaceUid", qNamespaceUID); err != nil {
				return err
			}
		}
	}

	// path param operatorVersion
	if err := r.SetPathParam("operatorVersion", o.OperatorVersion); err != nil {
		return err
	}

	if o.PhysicalBackupScheduled != nil {

		// query param physicalBackupScheduled
		var qrPhysicalBackupScheduled bool

		if o.PhysicalBackupScheduled != nil {
			qrPhysicalBackupScheduled = *o.PhysicalBackupScheduled
		}
		qPhysicalBackupScheduled := swag.FormatBool(qrPhysicalBackupScheduled)
		if qPhysicalBackupScheduled != "" {

			if err := r.SetQueryParam("physicalBackupScheduled", qPhysicalBackupScheduled); err != nil {
				return err
			}
		}
	}

	if o.PitrEnabled != nil {

		// query param pitrEnabled
		var qrPitrEnabled bool

		if o.PitrEnabled != nil {
			qrPitrEnabled = *o.PitrEnabled
		}
		qPitrEnabled := swag.FormatBool(qrPitrEnabled)
		if qPitrEnabled != "" {

			if err := r.SetQueryParam("pitrEnabled", qPitrEnabled); err != nil {
				return err
			}
		}
	}

	if o.Platform != nil {

		// query param platform
		var qrPlatform string

		if o.Platform != nil {
			qrPlatform = *o.Platform
		}
		qPlatform := qrPlatform
		if qPlatform != "" {

			if err := r.SetQueryParam("platform", qPlatform); err != nil {
				return err
			}
		}
	}

	if o.PmmEnabled != nil {

		// query param pmmEnabled
		var qrPmmEnabled bool

		if o.PmmEnabled != nil {
			qrPmmEnabled = *o.PmmEnabled
		}
		qPmmEnabled := swag.FormatBool(qrPmmEnabled)
		if qPmmEnabled != "" {

			if err := r.SetQueryParam("pmmEnabled", qPmmEnabled); err != nil {
				return err
			}
		}
	}

	if o.PmmVersion != nil {

		// query param pmmVersion
		var qrPmmVersion string

		if o.PmmVersion != nil {
			qrPmmVersion = *o.PmmVersion
		}
		qPmmVersion := qrPmmVersion
		if qPmmVersion != "" {

			if err := r.SetQueryParam("pmmVersion", qPmmVersion); err != nil {
				return err
			}
		}
	}

	// path param product
	if err := r.SetPathParam("product", o.Product); err != nil {
		return err
	}

	if o.ProxysqlVersion != nil {

		// query param proxysqlVersion
		var qrProxysqlVersion string

		if o.ProxysqlVersion != nil {
			qrProxysqlVersion = *o.ProxysqlVersion
		}
		qProxysqlVersion := qrProxysqlVersion
		if qProxysqlVersion != "" {

			if err := r.SetQueryParam("proxysqlVersion", qProxysqlVersion); err != nil {
				return err
			}
		}
	}

	if o.ShardingEnabled != nil {

		// query param shardingEnabled
		var qrShardingEnabled bool

		if o.ShardingEnabled != nil {
			qrShardingEnabled = *o.ShardingEnabled
		}
		qShardingEnabled := swag.FormatBool(qrShardingEnabled)
		if qShardingEnabled != "" {

			if err := r.SetQueryParam("shardingEnabled", qShardingEnabled); err != nil {
				return err
			}
		}
	}

	if o.SidecarsUsed != nil {

		// query param sidecarsUsed
		var qrSidecarsUsed bool

		if o.SidecarsUsed != nil {
			qrSidecarsUsed = *o.SidecarsUsed
		}
		qSidecarsUsed := swag.FormatBool(qrSidecarsUsed)
		if qSidecarsUsed != "" {

			if err := r.SetQueryParam("sidecarsUsed", qSidecarsUsed); err != nil {
				return err
			}
		}
	}

	if o.UserManagementEnabled != nil {

		// query param userManagementEnabled
		var qrUserManagementEnabled bool

		if o.UserManagementEnabled != nil {
			qrUserManagementEnabled = *o.UserManagementEnabled
		}
		qUserManagementEnabled := swag.FormatBool(qrUserManagementEnabled)
		if qUserManagementEnabled != "" {

			if err := r.SetQueryParam("userManagementEnabled", qUserManagementEnabled); err != nil {
				return err
			}
		}
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
