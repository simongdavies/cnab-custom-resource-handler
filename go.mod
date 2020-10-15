module github.com/simongdavies/cnab-custom-resource-handler

go 1.14

replace (
	// https://github.com/cnabio/cnab-go/pull/229 (valueset-schema)
	github.com/cnabio/cnab-go => github.com/carolynvs/cnab-go v0.13.4-0.20200820201933-d6bf372247e5
	// See https://github.com/containerd/containerd/issues/3031
	// When I try to just use the require, go is shortening it to v2.7.1+incompatible which then fails to build...
	github.com/docker/distribution => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309
	github.com/hashicorp/go-plugin => github.com/carolynvs/go-plugin v1.0.1-acceptstdin

)

require (
	get.porter.sh/porter v0.29.1
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.6
	github.com/Azure/go-autorest/autorest/adal v0.9.4
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.2
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.0 // indirect
	github.com/cnabio/cnab-go v0.13.4-0.20200817181428-9005c1da4354
	github.com/cnabio/cnab-to-oci v0.3.1-beta1
	github.com/containerd/containerd v1.3.0
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/cli v0.0.0-20191017083524-a8ff7f821017
	github.com/docker/distribution v2.7.1+incompatible
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-chi/render v1.0.1
	github.com/google/uuid v1.1.1
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v0.0.6
	github.com/stretchr/testify v1.6.1 // indirect
	golang.org/x/net v0.0.0-20200927032502-5d4f70055728 // indirect
	golang.org/x/sys v0.0.0-20200722175500-76b94024e4b6 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
)
