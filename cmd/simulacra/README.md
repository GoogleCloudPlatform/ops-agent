# Simulacra 

Simulacra is a command line tool that Ops Agent team members can use to spin up VMs with specific Ops Agent versions, third party applications, and any custom additional scripts.

Simulacra can be configured through command line for quick operations, or a config file for a reproducible setup. An Ops Agent config file can be passed in the Simulacra config and Simulacra can infer all the third party applications it needs to install from that file. 

## Example Usage

    > cat config.yaml
        logging:
            receivers:
                redis:
                    type: redis
            service:
                pipelines:
                    redis:
                        receivers:
                            - redis

    go run -tags=integration_test simulacra.go --ops_agent_config "/usr/local/google/home/shafinsiddique/ops-agent/integration_test/simulacra/config.yaml"


This command invokes Simulacra to spin up a VM, install the Ops Agent on it and then install the required third party applications. Based on the Ops Agent config in the example above, Simulacra will install Redis on the created VM. 

## Command Line Arguments

We offer a subset of the possible configuration parameters as command line arguments. This is for quick one-off usage. 

    --config : Optional. The path to a YAML file specifying all the configurations for Simulacra. If unspecified, Simulacra will either use values from other command line arguments or use default values. If specifed along with other command line arguments, all others will be ignored. 

    --platform: Optional. The platform for the VM. If missing, debian-11 is used.

    --ops_agent_config: Optional. Path to the Ops Agent Config File. If unspecified, Ops Agent will not install any third party applications and configure Ops Agent with default settings.

    --project: Optional. The project ID for the project where the VMs will be created. If missing, Simulacra will try to infer from GCloud config.

    --zone: Optional. The zone where the VM will be created. If missing, Simulacra will try to infer from GCloud config.

    --name: Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned.

    --third_party_apps_path: Optional. The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. Therefore, the default path is './integration_test/third_party_apps_data'

    --service_account: Optional. A service account for the VM. If missing, the VM will be instantiated with a default service account.




## YAML Config

| Key                      | Type           | Default | Description |
|:-------------------------|:---------------|:--------|:------------|
| `platform`            | string | debian-11 | The platform for the VM. |
| `ops_agent_config`             | string           | ""   | Path to the Ops Agent Config File. If unspecified, Ops Agent will not install any third party applications and configure Ops Agent with default settings. |
| `project`      | string           | ""   |  The project ID for the project where the VMs will be created. If missing, Simulacra will try to infer from GCloud config.|
| `zone`                | string       | ""     | The zone where the VM will be created. If missing, Simulacra will try to infer from GCloud config. |
| `name`                | string       | simulacra-vm-instance-<random_number> | A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned. |
| `third_party_apps_path`          | string       | "./integration_test/third_party_apps_data"     | The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. |
| `service_account`              | string | "" | A service account for the VM. If missing, the VM will be instantiated with a default service account. |
| `scripts`             | []Script       | []      | A list of scripts that will be executed on the VM. Useful for custom installations. See [Script](##Script) for more details. |

## Script

A script is a script file that will be executed on the VM. 

### Configuration 

| Key                      | Type           | Default | Description |
|:-------------------------|:---------------|:--------|:------------|
| `path`            | string | Required | Path to the script file. |
| `args`             | []string           | [] | A list of command line arguments that will be passed to the script. |