module github.com/openstack-k8s-operators/ironic-operator

go 1.19

require (
	github.com/go-logr/logr v1.2.4
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/onsi/ginkgo/v2 v2.11.0
	github.com/onsi/gomega v1.27.10
	github.com/openshift/api v3.9.0+incompatible
	github.com/openstack-k8s-operators/infra-operator/apis v0.1.0
	github.com/openstack-k8s-operators/ironic-operator/api v0.0.0-20230722140433-eba71851e2ff
	github.com/openstack-k8s-operators/keystone-operator/api v0.1.0
	github.com/openstack-k8s-operators/lib-common/modules/common v0.1.0
	github.com/openstack-k8s-operators/lib-common/modules/database v0.1.0
	github.com/openstack-k8s-operators/lib-common/modules/test v0.1.1
	github.com/openstack-k8s-operators/mariadb-operator/api v0.1.0
	k8s.io/api v0.26.7
	k8s.io/apimachinery v0.27.1
	k8s.io/client-go v0.26.7
	k8s.io/utils v0.0.0-20230711102312-30195339c3c7
	sigs.k8s.io/controller-runtime v0.14.6
)

require golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1 // indirect

require golang.org/x/mod v0.10.0 // indirect

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.1 // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/zapr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/uuid v1.3.0
	github.com/gophercloud/gophercloud v1.5.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect; indirect // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/openstack-k8s-operators/lib-common/modules/openstack v0.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.14.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.25.0
	golang.org/x/net v0.12.0 // indirect
	golang.org/x/oauth2 v0.0.0-20220909003341-f21342109be1 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/term v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.3 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/apiextensions-apiserver v0.26.7 // indirect
	k8s.io/component-base v0.26.7 // indirect
	k8s.io/klog/v2 v2.90.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230308215209-15aac26d736a // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace github.com/openstack-k8s-operators/ironic-operator/api => ./api

// mschuppert: map to latest commit from release-4.13 tag
// must consistent within modules and service operators
replace github.com/openshift/api => github.com/openshift/api v0.0.0-20230414143018-3367bc7e6ac7 //allow-merging

// This is need when updating to new version of lib-common.  For some reason, "go get -u" on the database and openstack
// lib-common modules breaks because they try to find this "v0.0.0-00010101000000-000000000000" common module version
// which does not exist.  So if you update the lib-common dependencies, uncomment the line below and change the common
// module version to the latest.  You can then comment this line out again once the database and openstack modules
// have been added.
// replace github.com/openstack-k8s-operators/lib-common/modules/common v0.0.0-00010101000000-000000000000 => github.com/openstack-k8s-operators/lib-common/modules/common v0.0.0-20220909175216-e774739df18a

// 1. push your code to github.com/fork/somelib @ dev branch
// 2. modify your go.mod file, add a line replace github.com/original/somelib => github.com/fork/somelib dev
// 3. execute go mod tidy command. After done these, go will auto replace the dev in go.mod to a suitiable pseudo-version.
// replace github.com/openstack-k8s-operators/lib-common => github.com/<account>/lib-common v0.0.0-20220610121238-abedf5879ca4
// replace github.com/openstack-k8s-operators/lib-common => /path/to/local/repo
