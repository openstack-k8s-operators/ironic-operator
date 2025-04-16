/*
Copyright 2022 Red Hat Inc.

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

// Package controllers contains shared utility functions for ironic-operator controllers.
package controllers

import (
	"context"
	"errors"
	"fmt"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Static errors for ironic controllers
var (
	ErrNetworkAttachmentConfig = errors.New("not all pods have interfaces with ips as configured in NetworkAttachments")
)

// fields to index to reconcile when change
const (
	passwordSecretField     = ".spec.secret"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName" // #nosec G101
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
	transportURLSecretField = ".spec.transportURLSecret"
	topologyField           = ".spec.topologyRef.Name"
)

var (
	ironicAPIWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
		tlsAPIInternalField,
		tlsAPIPublicField,
		transportURLSecretField,
		topologyField,
	}
	ironicConductorWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
		transportURLSecretField,
		topologyField,
	}
	ironicInspectorWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
		topologyField,
	}
	ironicNeutronAgentWatchFields = []string{
		passwordSecretField,
		caBundleSecretNameField,
		topologyField,
	}
)

func getCommonRbacRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid", "privileged"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}
}

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...interface{})
}

type topologyHandler interface {
	GetSpecTopologyRef() *topologyv1.TopoRef
	GetLastAppliedTopology() *topologyv1.TopoRef
	SetLastAppliedTopology(t *topologyv1.TopoRef)
}

func ensureTopology(
	ctx context.Context,
	helper *helper.Helper,
	instance topologyHandler,
	finalizer string,
	conditionUpdater conditionUpdater,
	defaultLabelSelector metav1.LabelSelector,
) (*topologyv1.Topology, error) {

	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		helper,
		instance.GetSpecTopologyRef(),
		instance.GetLastAppliedTopology(),
		finalizer,
		defaultLabelSelector,
	)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return nil, fmt.Errorf("waiting for Topology requirements: %w", err)
	}
	// update the Status with the last retrieved Topology (or set it to nil)
	instance.SetLastAppliedTopology(instance.GetSpecTopologyRef())
	// update the Topology condition only when a Topology is referenced and has
	// been retrieved (err == nil)
	if tr := instance.GetSpecTopologyRef(); tr != nil {
		// update the TopologyRef associated condition
		conditionUpdater.MarkTrue(
			condition.TopologyReadyCondition,
			condition.TopologyReadyMessage,
		)
	}
	return topology, nil
}

// getQuorumQueues - shared function to check for quorum queue setting in transportURL secret
func getQuorumQueues(
	ctx context.Context,
	h *helper.Helper,
	transportURLSecretName string,
	namespace string,
) (bool, error) {
	transportURLSecret, _, err := secret.GetSecret(ctx, h, transportURLSecretName, namespace)
	if err != nil {
		return false, err
	}
	quorumQueues := string(transportURLSecret.Data["quorumqueues"]) == "true"
	return quorumQueues, nil
}
