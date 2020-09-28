# cnab-arm-template-generator

Tool for generating ARM template from a CNAB bundle.

## Overview

This tool will generate an ARM template from a bundle definition, the resultant template will create a user assigned identity, assign contributor permission to the identity at the scope of the resource group that the template is deployed into, create a storage account , container and file share and then create an instance of the bundle using porter via the [deploymentScript](https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/template-tutorial-deployment-script) resource.

The generated template will contain a parameter for each parameter and credential that the bundle defines and will contain an output called `bundleOutput` that contains a JSON array containing the outputs from the bundle.

The template will also configure porter to use the azure storage plugin for state storage using the storage account defined in the template, if an installation already exists with the installation name then an install is performed, otherwise an upgrade is performed.

## Usage

### CLI

Generating the ARM template

```shell
Usage:
  cnabtoarmtemplate [flags]
  cnabtoarmtemplate [command]

Available Commands:
  getbundle   Gets Bundle file for a tag
  help        Help about any command
  listen      Starts an http server to listen for request for template generation
  version     Print the cnabtoarmtemplate version

Flags:
  -c, --customuidef         generates a custom createUIDefinition file called createUIdefinition.json in the same directory as the template
  -f, --file string         name of bundle file to generate template for , default is bundle.json in the current directory (default "bundle.json")
      --force               Force a fresh pull of the bundle
  -h, --help                help for cnabtoarmtemplate
  -i, --indent              specifies if the json output should be indented
      --insecure-registry   Don't require TLS for the registry
  -o, --output string       file name for generated template,default is azuredeploy.json (default "azuredeploy.json")
      --overwrite           specifies if to overwrite the output file if it already exists, default is false
  -r, --replace             specifies if the ARM template generated should replace Kubeconfig Parameters with AKS references
  -s, --simplify            specifies if the ARM template should be simplified, exposing less parameters and inferring default values
  -t, --tag string          Use a bundle specified by the given tag.
      --timeout int         specifies the time in minutes that is allowed for execution of the CNAB Action in the generated template (default 15)
```