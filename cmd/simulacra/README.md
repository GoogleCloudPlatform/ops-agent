# Simulacra 

Simulacra is a command line tool that the oncall team can use to automatically spin up VMs with a customers environment. 

Simulacra can read Ops Agent config files and infer all the third party apps that it needs to install. It then instantiates the VM according to user specifications, installs Ops Agent and then installs all the required third party applications. 
gi
We can specify various configurations for our desired VM. For example, we can specify the Operating System we want our VM to have. 

Ultimately, we want to integrate Simulacra with our diagnostic tool and be able to generate Simulacra config files from the output of the diagnostic tool.  

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


This command invokes Simulacra to spin up a VM, install Ops Agent on it and then install the required third party applications. Based on the ops agent config file, the only third party application Ops Agent is configured with is Redis so Simulacra will only install Redis. 


## Command Line Arguments

For any advanced usage, Simulacra should be configured using a YAML file and then invoked by specifying the --config argument. 

However, we also offer a subset of the total config parameters as command line arguments. This is for quick usages. 

    --config : Optional. The path to a YAML file specifying all the configurations for Simulacra. If unspecified, Simulacra will either use values from other command line arguments or use default values. If specifed along with other command line arguments, all others will be ignored. 

    --platform: Optional. The OS for the VM. If missing, debian-11 is used.

    --ops_agent_config: Optional. Path to the Ops Agent Config File. If unspecified, Ops Agent will not install any third party applications and configure Ops Agent with default settings.

    --project: Optional. The project ID for the project that will be used to instantiate the VMs. If missing, Simulacra will try to infer from GCloud config.

    --zone: Optional. The zone in which the VM will be instantiated in. If missing, Simulacra will try to infer from GCloud config.

    --name: Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned.

    --third_party_apps_path: Optional. The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. Therefore, the default path is './integration_test/third_party_apps_data'


## YAML Config

As mentioned above, Simulacra can be configured by creating a YAML file and then specifying the '--config' command line argument. 

Using YAML files, we can get even more options for configuring our VMs than just using command line arguments. 


    - platform (String): Optional. The OS for the VM. If missing, debian-11 is used.

    -- ops_agent_config (String): Optional. The OS for the VM. If missing, debian-11 is used.

    - project (String): Optional. The project ID for the project that will be used to instantiate the VMs. If missing, Simulacra will try to infer from GCloud config.

    --zone (String): Optional. The zone in which the VM will be instantiated in. If missing, Simulacra will try to infer from GCloud config.

    - name (String): Optional. A name for the instance to be created. If missing, a random name with prefix 'simulacra-vm-instance' will be assigned.

    - third_party_apps_path (String): Optional. The path to the third party apps data folder. If missing, Simulacra assumes the working directory is the root of the repo. Therefore, the default path is './integration_test/third_party_apps_data'

    - scripts ([] String): A list of paths to script files. This is if we want to execute certain scripts after the VM executes. Useful for custom installations. 
