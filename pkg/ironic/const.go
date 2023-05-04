/*

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

package ironic

const (
	// ServiceName -
	ServiceName = "ironic"
	// ServiceType -
	ServiceType = "baremetal"
	// ServiceAccount -
	ServiceAccount = "ironic-operator-ironic"
	// DatabaseName -
	DatabaseName = "ironic"
	// IronicPublicPort -
	IronicPublicPort int32 = 6385
	// IronicInternalPort -
	IronicInternalPort int32 = 6385
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// ComponentSelector - used by operators to specify pod labels
	ComponentSelector = "component"
	// ConductorComponent -
	ConductorComponent = "conductor"
	// HttpbootComponent -
	HttpbootComponent = "httpboot"
	// JSONRPCComponent -
	JSONRPCComponent = "jsonrpc"
	// DhcpComponent -
	DhcpComponent = "dhcp"
	// APIComponent -
	APIComponent = "api"
	// InspectorComponent -
	InspectorComponent = "inspector"
	// ConductorGroupSelector -
	ConductorGroupSelector = "conductorGroup"
)
