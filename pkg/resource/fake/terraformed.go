// SPDX-FileCopyrightText: 2023 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
)

// Observable is mock Observable.
type Observable struct {
	Observation                 map[string]any
	SetObservationStub          func(data map[string]any) error
	AdditionalConnectionDetails map[string][]byte
	ID                          string
}

// GetObservation is a mock.
func (o *Observable) GetObservation() (map[string]any, error) {
	return o.Observation, nil
}

// SetObservation is a mock.
func (o *Observable) SetObservation(data map[string]any) error {
	if o.SetObservationStub != nil {
		return o.SetObservationStub(data)
	}

	o.Observation = data
	return nil
}

// GetID is a mock.
func (o *Observable) GetID() string {
	return o.ID
}

// GetAdditionalConnectionDetails is a mock
func (o *Observable) GetAdditionalConnectionDetails(_ map[string]any) (map[string][]byte, error) {
	return o.AdditionalConnectionDetails, nil
}

// Parameterizable is mock Parameterizable.
type Parameterizable struct {
	Parameters     map[string]any
	InitParameters map[string]any
}

func (t *Terraformed) GetMergedParameters(_ bool) (map[string]any, error) {
	return t.Parameters, nil
}

// GetParameters is a mock.
func (p *Parameterizable) GetParameters() (map[string]any, error) {
	return p.Parameters, nil
}

// SetParameters is a mock.
func (p *Parameterizable) SetParameters(data map[string]any) error {
	p.Parameters = data
	return nil
}

// GetInitParameters is a mock.
func (p *Parameterizable) GetInitParameters() (map[string]any, error) {
	return p.InitParameters, nil
}

// MetadataProvider is mock MetadataProvider.
type MetadataProvider struct {
	Type                     string
	SchemaVersion            int
	ConnectionDetailsMapping map[string]string
}

// GetTerraformResourceType is a mock.
func (mp *MetadataProvider) GetTerraformResourceType() string {
	return mp.Type
}

// GetTerraformSchemaVersion is a mock.
func (mp *MetadataProvider) GetTerraformSchemaVersion() int {
	return mp.SchemaVersion
}

// GetConnectionDetailsMapping is a mock.
func (mp *MetadataProvider) GetConnectionDetailsMapping() map[string]string {
	return mp.ConnectionDetailsMapping
}

// LateInitializer is mock LateInitializer.
type LateInitializer struct {
	Result bool
	Err    error
}

// LateInitialize is a mock.
func (li *LateInitializer) LateInitialize(_ []byte) (bool, error) {
	return li.Result, li.Err
}

// Terraformed is a mock that implements Terraformed interface.
type Terraformed struct {
	fake.Managed
	Observable
	Parameterizable
	MetadataProvider
	LateInitializer
}

// GetObjectKind returns schema.ObjectKind.
func (t *Terraformed) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (t *Terraformed) DeepCopyObject() runtime.Object {
	out := &Terraformed{}
	j, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

// Option is an option to modify the properties of a Terraformed object.
type Option func(terraformed *Terraformed)

// WithParameters sets the parameters of a Terraformed.
func WithParameters(params map[string]any) Option {
	return func(tr *Terraformed) {
		tr.Parameters = params
	}
}

// NewTerraformed initializes a new Terraformed with the given options.
func NewTerraformed(opts ...Option) *Terraformed {
	tr := &Terraformed{}
	for _, o := range opts {
		o(tr)
	}
	return tr
}

// NewMap prepares a map from the supplied key value parameters.
// The parameters slice must be a sequence of key, value pairs and must have
// an even length. The function will panic otherwise.
func NewMap(keyValue ...string) map[string]any {
	m := make(map[string]any, len(keyValue)/2)
	for i := 0; i < len(keyValue)-1; i += 2 {
		m[keyValue[i]] = keyValue[i+1]
	}
	return m
}
