// Code generated by go-swagger; DO NOT EDIT.

package version_service

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/percona/percona-xtradb-cluster-operator/version/client/models"
)

// VersionServiceMetadataV2Reader is a Reader for the VersionServiceMetadataV2 structure.
type VersionServiceMetadataV2Reader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *VersionServiceMetadataV2Reader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewVersionServiceMetadataV2OK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewVersionServiceMetadataV2Default(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewVersionServiceMetadataV2OK creates a VersionServiceMetadataV2OK with default headers values
func NewVersionServiceMetadataV2OK() *VersionServiceMetadataV2OK {
	return &VersionServiceMetadataV2OK{}
}

/*
VersionServiceMetadataV2OK describes a response with status code 200, with default header values.

A successful response.
*/
type VersionServiceMetadataV2OK struct {
	Payload *models.VersionMetadataV2Response
}

// IsSuccess returns true when this version service metadata v2 o k response has a 2xx status code
func (o *VersionServiceMetadataV2OK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this version service metadata v2 o k response has a 3xx status code
func (o *VersionServiceMetadataV2OK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this version service metadata v2 o k response has a 4xx status code
func (o *VersionServiceMetadataV2OK) IsClientError() bool {
	return false
}

// IsServerError returns true when this version service metadata v2 o k response has a 5xx status code
func (o *VersionServiceMetadataV2OK) IsServerError() bool {
	return false
}

// IsCode returns true when this version service metadata v2 o k response a status code equal to that given
func (o *VersionServiceMetadataV2OK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the version service metadata v2 o k response
func (o *VersionServiceMetadataV2OK) Code() int {
	return 200
}

func (o *VersionServiceMetadataV2OK) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /metadata/v2/{product}][%d] versionServiceMetadataV2OK %s", 200, payload)
}

func (o *VersionServiceMetadataV2OK) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /metadata/v2/{product}][%d] versionServiceMetadataV2OK %s", 200, payload)
}

func (o *VersionServiceMetadataV2OK) GetPayload() *models.VersionMetadataV2Response {
	return o.Payload
}

func (o *VersionServiceMetadataV2OK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.VersionMetadataV2Response)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewVersionServiceMetadataV2Default creates a VersionServiceMetadataV2Default with default headers values
func NewVersionServiceMetadataV2Default(code int) *VersionServiceMetadataV2Default {
	return &VersionServiceMetadataV2Default{
		_statusCode: code,
	}
}

/*
VersionServiceMetadataV2Default describes a response with status code -1, with default header values.

An unexpected error response.
*/
type VersionServiceMetadataV2Default struct {
	_statusCode int

	Payload *models.GooglerpcStatus
}

// IsSuccess returns true when this version service metadata v2 default response has a 2xx status code
func (o *VersionServiceMetadataV2Default) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this version service metadata v2 default response has a 3xx status code
func (o *VersionServiceMetadataV2Default) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this version service metadata v2 default response has a 4xx status code
func (o *VersionServiceMetadataV2Default) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this version service metadata v2 default response has a 5xx status code
func (o *VersionServiceMetadataV2Default) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this version service metadata v2 default response a status code equal to that given
func (o *VersionServiceMetadataV2Default) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the version service metadata v2 default response
func (o *VersionServiceMetadataV2Default) Code() int {
	return o._statusCode
}

func (o *VersionServiceMetadataV2Default) Error() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /metadata/v2/{product}][%d] VersionService_MetadataV2 default %s", o._statusCode, payload)
}

func (o *VersionServiceMetadataV2Default) String() string {
	payload, _ := json.Marshal(o.Payload)
	return fmt.Sprintf("[GET /metadata/v2/{product}][%d] VersionService_MetadataV2 default %s", o._statusCode, payload)
}

func (o *VersionServiceMetadataV2Default) GetPayload() *models.GooglerpcStatus {
	return o.Payload
}

func (o *VersionServiceMetadataV2Default) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.GooglerpcStatus)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}