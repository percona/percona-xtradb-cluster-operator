// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// VersionMetadataResponse version metadata response
//
// swagger:model versionMetadataResponse
type VersionMetadataResponse struct {

	// versions
	Versions []*VersionMetadataVersion `json:"versions"`
}

// Validate validates this version metadata response
func (m *VersionMetadataResponse) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateVersions(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *VersionMetadataResponse) validateVersions(formats strfmt.Registry) error {
	if swag.IsZero(m.Versions) { // not required
		return nil
	}

	for i := 0; i < len(m.Versions); i++ {
		if swag.IsZero(m.Versions[i]) { // not required
			continue
		}

		if m.Versions[i] != nil {
			if err := m.Versions[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("versions" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("versions" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this version metadata response based on the context it is used
func (m *VersionMetadataResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateVersions(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *VersionMetadataResponse) contextValidateVersions(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.Versions); i++ {

		if m.Versions[i] != nil {

			if swag.IsZero(m.Versions[i]) { // not required
				return nil
			}

			if err := m.Versions[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("versions" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("versions" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *VersionMetadataResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *VersionMetadataResponse) UnmarshalBinary(b []byte) error {
	var res VersionMetadataResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
