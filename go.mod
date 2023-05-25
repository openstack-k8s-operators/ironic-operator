module github.com/openstack-k8s-operators/ironic-operator

go 1.19

require (
	github.com/go-logr/logr v1.2.4
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/onsi/ginkgo/v2 v2.9.5
	github.com/onsi/gomega v1.27.7
	github.com/openshift/api v3.9.0+incompatible
	github.com/openstack-k8s-operators/infra-operator/apis v0.0.0-20230524091321-cad234ed99a6
	github.com/openstack-k8s-operators/ironic-operator/api v0.0.0-20230523205452-0215cd6c6947
	github.com/openstack-k8s-operators/keystone-operator/api v0.0.0-20230524090129-769ebc6d0776
	github.com/openstack-k8s-operators/lib-common/modules/common v0.0.0-20230523095909-db05945b0b1e
	github.com/openstack-k8s-operators/lib-common/modules/database v0.0.0-20230523095909-db05945b0b1e
	github.com/openstack-k8s-operators/lib-common/modules/test-operators v0.0.0-20230523095909-db05945b0b1e
	github.com/openstack-k8s-operators/mariadb-operator/api v0.0.0-20230523102317-8b60f83b1d1f
	k8s.io/api v0.27.2
	k8s.io/apimachinery v0.27.2
	k8s.io/client-go v0.27.2
	k8s.io/utils v0.0.0-20230313181309-38a27ef9d749
	sigs.k8s.io/controller-runtime v0.15.0
)

require (
	github.com/openstack-k8s-operators/lib-common/modules/test v0.0.0-20230523095909-db05945b0b1e
	golang.org/x/mod v0.10.0 // indirect
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.1 // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/zapr v1.2.4 // indirect
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
	github.com/gophercloud/gophercloud v1.2.0 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/openstack-k8s-operators/lib-common/modules/openstack v0.0.0-20230523095909-db05945b0b1e // indirect; indirect // indirect // indirect // indirect // indirect // indirect // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.15.1 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.24.0
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/oauth2 v0.5.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/term v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/apiextensions-apiserver v0.27.2 // indirect
	k8s.io/component-base v0.27.2 // indirect
	k8s.io/klog/v2 v2.90.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace github.com/openstack-k8s-operators/ironic-operator/api => ./api

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
