// SPDX-FileCopyrightText: 2023 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/crossplane/upjet/pkg/resource/json"
	"github.com/crossplane/upjet/pkg/schema/traverser"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	tf "github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/upjet/pkg/config"
	"github.com/crossplane/upjet/pkg/resource/fake"
	"github.com/crossplane/upjet/pkg/terraform"
)

var (
	zl      = zap.New(zap.UseDevMode(true))
	logTest = logging.NewLogrLogger(zl.WithName("provider-aws"))
	ots     = NewOperationStore(logTest)
	timeout = time.Duration(1200000000000)
	cfg     = &config.Resource{
		TerraformResource: &schema.Resource{
			Timeouts: &schema.ResourceTimeout{
				Create: &timeout,
				Read:   &timeout,
				Update: &timeout,
				Delete: &timeout,
			},
			Schema: map[string]*schema.Schema{
				"name": {
					Type:     schema.TypeString,
					Required: true,
				},
				"id": {
					Type:     schema.TypeString,
					Computed: true,
					Required: false,
				},
				"map": {
					Type: schema.TypeMap,
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
				"list": {
					Type: schema.TypeList,
					Elem: &schema.Schema{
						Type: schema.TypeString,
					},
				},
			},
		},
		ExternalName: config.IdentifierFromProvider,
		Sensitive: config.Sensitive{AdditionalConnectionDetailsFn: func(attr map[string]any) (map[string][]byte, error) {
			return nil, nil
		}},
	}
	obj = fake.Terraformed{
		Parameterizable: fake.Parameterizable{
			Parameters: map[string]any{
				"name": "example",
				"map": map[string]any{
					"key": "value",
				},
				"list": []any{"elem1", "elem2"},
			},
		},
		Observable: fake.Observable{
			Observation: map[string]any{},
		},
	}
)

func prepareTerraformPluginSDKExternal(r Resource, cfg *config.Resource) *terraformPluginSDKExternal {
	schemaBlock := cfg.TerraformResource.CoreConfigSchema()
	rawConfig, err := schema.JSONMapToStateValue(map[string]any{"name": "example"}, schemaBlock)
	if err != nil {
		panic(err)
	}
	return &terraformPluginSDKExternal{
		ts:             terraform.Setup{},
		resourceSchema: r,
		config:         cfg,
		params: map[string]any{
			"name": "example",
		},
		rawConfig: rawConfig,
		logger:    logTest,
		opTracker: NewAsyncTracker(),
	}
}

type mockResource struct {
	ApplyFn                 func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics)
	RefreshWithoutUpgradeFn func(ctx context.Context, s *tf.InstanceState, meta interface{}) (*tf.InstanceState, diag.Diagnostics)
}

func (m mockResource) Apply(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
	return m.ApplyFn(ctx, s, d, meta)
}

func (m mockResource) RefreshWithoutUpgrade(ctx context.Context, s *tf.InstanceState, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
	return m.RefreshWithoutUpgradeFn(ctx, s, meta)
}

func TestTerraformPluginSDKConnect(t *testing.T) {
	type args struct {
		setupFn terraform.SetupFn
		cfg     *config.Resource
		ots     *OperationTrackerStore
		obj     fake.Terraformed
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				setupFn: func(_ context.Context, _ client.Client, _ xpresource.Managed) (terraform.Setup, error) {
					return terraform.Setup{}, nil
				},
				cfg: cfg,
				obj: obj,
				ots: ots,
			},
		},
		"HCL": {
			args: args{
				setupFn: func(_ context.Context, _ client.Client, _ xpresource.Managed) (terraform.Setup, error) {
					return terraform.Setup{}, nil
				},
				cfg: cfg,
				obj: fake.Terraformed{
					Parameterizable: fake.Parameterizable{
						Parameters: map[string]any{
							"name": "      ${jsonencode({\n          type = \"object\"\n        })}",
							"map": map[string]any{
								"key": "value",
							},
							"list": []any{"elem1", "elem2"},
						},
					},
					Observable: fake.Observable{
						Observation: map[string]any{},
					},
				},
				ots: ots,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := NewTerraformPluginSDKConnector(nil, tc.args.setupFn, tc.args.cfg, tc.args.ots, WithTerraformPluginSDKLogger(logTest))
			_, err := c.Connect(context.TODO(), &tc.args.obj)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nConnect(...): -want error, +got error:\n", diff)
			}
		})
	}
}

func TestTerraformPluginSDKObserve(t *testing.T) {
	type args struct {
		r   Resource
		cfg *config.Resource
		obj fake.Terraformed
	}
	type want struct {
		obs managed.ExternalObservation
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"NotExists": {
			args: args{
				r: mockResource{
					RefreshWithoutUpgradeFn: func(ctx context.Context, s *tf.InstanceState, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return nil, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:          false,
					ResourceUpToDate:        false,
					ResourceLateInitialized: false,
					ConnectionDetails:       nil,
					Diff:                    "",
				},
			},
		},
		"UpToDate": {
			args: args{
				r: mockResource{
					RefreshWithoutUpgradeFn: func(ctx context.Context, s *tf.InstanceState, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id", Attributes: map[string]string{"name": "example"}}, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       nil,
					Diff:                    "",
				},
			},
		},
		"InitProvider": {
			args: args{
				r: mockResource{
					RefreshWithoutUpgradeFn: func(ctx context.Context, s *tf.InstanceState, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id", Attributes: map[string]string{"name": "example2"}}, nil
					},
				},
				cfg: cfg,
				obj: fake.Terraformed{
					Parameterizable: fake.Parameterizable{
						Parameters: map[string]any{
							"name": "example",
							"map": map[string]any{
								"key": "value",
							},
							"list": []any{"elem1", "elem2"},
						},
						InitParameters: map[string]any{
							"list": []any{"elem1", "elem2", "elem3"},
						},
					},
					Observable: fake.Observable{
						Observation: map[string]any{},
					},
				},
			},
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       nil,
					Diff:                    "",
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			terraformPluginSDKExternal := prepareTerraformPluginSDKExternal(tc.args.r, tc.args.cfg)
			observation, err := terraformPluginSDKExternal.Observe(context.TODO(), &tc.args.obj)
			if diff := cmp.Diff(tc.want.obs, observation); diff != "" {
				t.Errorf("\n%s\nObserve(...): -want observation, +got observation:\n", diff)
			}
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nConnect(...): -want error, +got error:\n", diff)
			}
		})
	}
}

func TestTerraformPluginSDKCreate(t *testing.T) {
	type args struct {
		r   Resource
		cfg *config.Resource
		obj fake.Terraformed
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"Unsuccessful": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return nil, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
			want: want{
				err: errors.New("failed to read the ID of the new resource"),
			},
		},
		"Successful": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id"}, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			terraformPluginSDKExternal := prepareTerraformPluginSDKExternal(tc.args.r, tc.args.cfg)
			_, err := terraformPluginSDKExternal.Create(context.TODO(), &tc.args.obj)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nConnect(...): -want error, +got error:\n", diff)
			}
		})
	}
}

func TestTerraformPluginSDKUpdate(t *testing.T) {
	type args struct {
		r   Resource
		cfg *config.Resource
		obj fake.Terraformed
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id"}, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			terraformPluginSDKExternal := prepareTerraformPluginSDKExternal(tc.args.r, tc.args.cfg)
			_, err := terraformPluginSDKExternal.Update(context.TODO(), &tc.args.obj)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nConnect(...): -want error, +got error:\n", diff)
			}
		})
	}
}

func TestTerraformPluginSDKUpdateSingletonList(t *testing.T) {
	type args struct {
		r   Resource
		cfg *config.Resource
		obj fake.Terraformed
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"WithoutConversions - Fail": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id",
							Attributes: map[string]string{
								"name":        "example",
								"list.#":      "1",
								"list.0.item": "elem1",
							}}, nil
					},
				},
				cfg: newSingletonListResource(nil),
				obj: fake.Terraformed{
					Parameterizable: fake.Parameterizable{
						Parameters: map[string]any{
							"name": "example",
							"list": map[string]interface{}{"item": "elem1"},
						},
					},
					Observable: fake.Observable{
						Observation: map[string]any{},
						SetObservationStub: func(data map[string]any) error {
							p, err := json.TFParser.Marshal(data)
							if err != nil {
								return err
							}

							obs := struct {
								Name string `json:"name,omitempty" tf:"name,omitempty"`
								List *struct {
									Item *string `json:"item,omitempty" tf:"item,omitempty"`
								} `json:"list,omitempty" tf:"list,omitempty"`
							}{}

							return json.TFParser.Unmarshal(p, &obs)
						},
					},
				},
			},
			want: want{
				err: errors.New("failed to set observation: "),
			},
		},
		"WithConversion - Success": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id",
							Attributes: map[string]string{
								"name":        "example",
								"list.#":      "1",
								"list.0.item": "elem1",
							}}, nil
					},
				},
				cfg: newSingletonListResource([]config.TerraformConversion{config.NewTFSingletonConversion()}),
				obj: fake.Terraformed{
					Parameterizable: fake.Parameterizable{
						Parameters: map[string]any{
							"name": "example",
							"list": map[string]interface{}{"item": "elem1"},
						},
					},
					Observable: fake.Observable{
						Observation: map[string]any{},
						SetObservationStub: func(data map[string]any) error {
							p, err := json.TFParser.Marshal(data)
							if err != nil {
								return err
							}

							obs := struct {
								Name string `json:"name,omitempty" tf:"name,omitempty"`
								List *struct {
									Item *string `json:"item,omitempty" tf:"item,omitempty"`
								} `json:"list,omitempty" tf:"list,omitempty"`
							}{}

							return json.TFParser.Unmarshal(p, &obs)
						},
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			listEmbedder := &config.SingletonListEmbedder{}
			listEmbedder.SetResource(tc.args.cfg)
			if err := traverser.TFResourceSchema(map[string]*schema.Resource{"test": tc.args.cfg.TerraformResource}).Traverse(listEmbedder); err != nil {
				t.Errorf("cannot sync the MaxItems constraints between the Go schema and the JSON schema")
			}

			terraformPluginSDKExternal := prepareTerraformPluginSDKExternal(tc.args.r, tc.args.cfg)
			_, err := terraformPluginSDKExternal.Update(context.TODO(), &tc.args.obj)
			if tc.want.err == nil && err != nil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.want.err.Error())
			}
		})
	}
}

func newSingletonListResource(tfConversions []config.TerraformConversion) *config.Resource {
	tfResource := &schema.Resource{
		Timeouts: &schema.ResourceTimeout{
			Create: &timeout,
			Read:   &timeout,
			Update: &timeout,
			Delete: &timeout,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"list": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"item": {
							Type: schema.TypeString,
						},
					},
				},
			},
		},
	}
	res := config.DefaultResource("aws_singleton_list", tfResource, nil, nil)
	res.TerraformConversions = tfConversions
	return res
}

func TestTerraformPluginSDKDelete(t *testing.T) {
	type args struct {
		r   Resource
		cfg *config.Resource
		obj fake.Terraformed
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"Successful": {
			args: args{
				r: mockResource{
					ApplyFn: func(ctx context.Context, s *tf.InstanceState, d *tf.InstanceDiff, meta interface{}) (*tf.InstanceState, diag.Diagnostics) {
						return &tf.InstanceState{ID: "example-id"}, nil
					},
				},
				cfg: cfg,
				obj: obj,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			terraformPluginSDKExternal := prepareTerraformPluginSDKExternal(tc.args.r, tc.args.cfg)
			err := terraformPluginSDKExternal.Delete(context.TODO(), &tc.args.obj)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nConnect(...): -want error, +got error:\n", diff)
			}
		})
	}
}
