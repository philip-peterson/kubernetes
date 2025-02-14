/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validatingadmissionpolicy

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"k8s.io/api/admissionregistration/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/cel/openapi/resolver"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

func TestExtractTypeNames(t *testing.T) {
	for _, tc := range []struct {
		name     string
		policy   *v1alpha1.ValidatingAdmissionPolicy
		expected []schema.GroupVersionKind // must be sorted
	}{
		{
			name:     "empty",
			policy:   &v1alpha1.ValidatingAdmissionPolicy{},
			expected: nil,
		},
		{
			name: "specific",
			policy: &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"v1"},
								Resources:   []string{"deployments"},
							},
						},
					},
				}},
			}},
			expected: []schema.GroupVersionKind{{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			}},
		},
		{
			name: "multiple",
			policy: &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"v1"},
								Resources:   []string{"deployments"},
							},
						},
					}, {
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
							},
						},
					},
				}},
			}},
			expected: []schema.GroupVersionKind{
				{
					Version: "v1",
					Kind:    "Pod",
				}, {
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}},
		},
		{
			name: "all resources",
			policy: &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"v1"},
								Resources:   []string{"*"},
							},
						},
					},
				}},
			}},
			expected: nil,
		},
		{
			name: "sub resources",
			policy: &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods/*"},
							},
						},
					},
				}},
			}},
			expected: nil,
		},
		{
			name: "mixtures",
			policy: &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
				MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"v1"},
								Resources:   []string{"deployments"},
							},
						},
					},
					{
						RuleWithOperations: v1alpha1.RuleWithOperations{
							Rule: v1alpha1.Rule{
								APIGroups:   []string{"apps"},
								APIVersions: []string{"*"},
								Resources:   []string{"deployments"},
							},
						},
					},
				}},
			}},
			expected: []schema.GroupVersionKind{{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typeChecker := buildTypeChecker(nil)
			got := typeChecker.typesToCheck(tc.policy)
			if !reflect.DeepEqual(tc.expected, got) {
				t.Errorf("expected %v but got %v", tc.expected, got)
			}
		})
	}
}

func TestTypeCheck(t *testing.T) {
	deploymentPolicy := &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
		Validations: []v1alpha1.Validation{
			{
				Expression: "object.foo == 'bar'",
			},
		},
		MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
			{
				RuleWithOperations: v1alpha1.RuleWithOperations{
					Rule: v1alpha1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				},
			},
		}},
	}}

	deploymentPolicyWithBadMessageExpression := deploymentPolicy.DeepCopy()
	deploymentPolicyWithBadMessageExpression.Spec.Validations[0].MessageExpression = "object.foo + 114514" // confusion

	multiExpressionPolicy := &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
		Validations: []v1alpha1.Validation{
			{
				Expression: "object.foo == 'bar'",
			},
			{
				Expression: "object.bar == 'foo'",
			},
		},
		MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
			{
				RuleWithOperations: v1alpha1.RuleWithOperations{
					Rule: v1alpha1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				},
			},
		}},
	}}
	paramsRefPolicy := &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
		ParamKind: &v1alpha1.ParamKind{
			APIVersion: "v1",
			Kind:       "DoesNotMatter",
		},
		Validations: []v1alpha1.Validation{
			{
				Expression: "object.foo == params.bar",
			},
		},
		MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
			{
				RuleWithOperations: v1alpha1.RuleWithOperations{
					Rule: v1alpha1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				},
			},
		}},
	}}
	authorizerPolicy := &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
		Validations: []v1alpha1.Validation{
			{
				Expression: "authorizer.group('').resource('endpoints').check('create').allowed()",
			},
		},
		MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
			{
				RuleWithOperations: v1alpha1.RuleWithOperations{
					Rule: v1alpha1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				},
			},
		}},
	}}
	authorizerInvalidPolicy := &v1alpha1.ValidatingAdmissionPolicy{Spec: v1alpha1.ValidatingAdmissionPolicySpec{
		Validations: []v1alpha1.Validation{
			{
				Expression: "authorizer.allowed()",
			},
		},
		MatchConstraints: &v1alpha1.MatchResources{ResourceRules: []v1alpha1.NamedRuleWithOperations{
			{
				RuleWithOperations: v1alpha1.RuleWithOperations{
					Rule: v1alpha1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				},
			},
		}},
	}}
	for _, tc := range []struct {
		name           string
		schemaToReturn *spec.Schema
		policy         *v1alpha1.ValidatingAdmissionPolicy
		assertions     []assertionFunc
	}{
		{
			name:       "empty",
			policy:     &v1alpha1.ValidatingAdmissionPolicy{},
			assertions: []assertionFunc{toBeEmpty},
		},
		{
			name:           "unresolved schema",
			policy:         deploymentPolicy,
			schemaToReturn: nil,
			assertions:     []assertionFunc{toBeEmpty},
		},
		{
			name:   "passed check",
			policy: deploymentPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{toBeEmpty},
		},
		{
			name:   "undefined field",
			policy: deploymentPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"bar": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[0].expression"),
				toContain(`undefined field 'foo'`),
			},
		},
		{
			name:   "field type mismatch",
			policy: deploymentPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.Int64Property(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[0].expression"),
				toContain(`found no matching overload`),
			},
		},
		{
			name:   "params",
			policy: paramsRefPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[0].expression"),
				toContain(`undefined field 'bar'`),
			},
		},
		{
			name:   "multiple expressions",
			policy: multiExpressionPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[1].expression"), // expressions[0] is okay, [1] is wrong
				toHaveLengthOf(1),
			},
		},
		{
			name:   "message expressions",
			policy: deploymentPolicyWithBadMessageExpression,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[0].messageExpression"),
				toHaveLengthOf(1),
			},
		},
		{
			name:   "authorizer",
			policy: authorizerPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{toBeEmpty},
		},
		{
			name:   "authorizer invalid",
			policy: authorizerInvalidPolicy,
			schemaToReturn: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"foo": *spec.StringProperty(),
					},
				},
			},
			assertions: []assertionFunc{
				toHaveFieldRef("spec.validations[0].expression"),
				toHaveLengthOf(1),
				toContain("found no matching overload for 'allowed' applied to 'kubernetes.authorization.Authorizer"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typeChecker := buildTypeChecker(tc.schemaToReturn)
			warnings := typeChecker.Check(tc.policy)
			for _, a := range tc.assertions {
				a(warnings, t)
			}
		})
	}
}

func buildTypeChecker(schemaToReturn *spec.Schema) *TypeChecker {
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{
			Group:   "",
			Version: "v1",
		},
	})
	restMapper.Add(must3(scheme.ObjectKinds(&corev1.Pod{}))[0], meta.RESTScopeRoot)
	restMapper.Add(must3(scheme.ObjectKinds(&appsv1.Deployment{}))[0], meta.RESTScopeRoot)

	return &TypeChecker{
		SchemaResolver: &fakeSchemaResolver{schemaToReturn: schemaToReturn},
		RestMapper:     restMapper,
	}
}

type fakeSchemaResolver struct {
	schemaToReturn *spec.Schema
}

func (r *fakeSchemaResolver) ResolveSchema(gvk schema.GroupVersionKind) (*spec.Schema, error) {
	if r.schemaToReturn == nil {
		return nil, fmt.Errorf("cannot resolve for %v: %w", gvk, resolver.ErrSchemaNotFound)
	}
	return r.schemaToReturn, nil
}

func toBeEmpty(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
	if len(warnings) != 0 {
		t.Fatalf("expected empty but got %v", warnings)
	}
}

func toContain(substring string) func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
	return func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
		if len(warnings) == 0 {
			t.Errorf("expected containing %q but got empty", substring)
		}
		for i, w := range warnings {
			if !strings.Contains(w.Warning, substring) {
				t.Errorf("warning %d does not contain %q, got %v", i, substring, w)
			}
		}
	}
}

func toHaveLengthOf(expected int) func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
	return func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
		got := len(warnings)
		if expected != got {
			t.Errorf("expect warnings to have length of %d, but got %d", expected, got)
		}
	}
}

func toHaveFieldRef(paths ...string) func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
	return func(warnings []v1alpha1.ExpressionWarning, t *testing.T) {
		if len(paths) != len(warnings) {
			t.Errorf("expect warnings to have length of %d, but got %d", len(paths), len(warnings))
		}
		for i := range paths {
			if paths[i] != warnings[i].FieldRef {
				t.Errorf("wrong fieldRef at %d, expected %q but got %q", i, paths[i], warnings[i].FieldRef)
			}
		}
	}
}

type assertionFunc func(warnings []v1alpha1.ExpressionWarning, t *testing.T)
