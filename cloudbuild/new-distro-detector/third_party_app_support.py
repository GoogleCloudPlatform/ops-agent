# This script parses a 'project.yaml' file from the ops-agent-build-release repository.
# It extracts information about the supported operating system distributions
# and their package management types.
#
# To run this script:
# 1. Ensure you have the 'pyyaml' library installed: pip install pyyaml
# 2. Place a copy of the 'project.yaml' file in the same directory as this script.
# 3. Run the script from your terminal: python ops_agent_parser.py

import yaml
import os
from dataclasses import dataclass
from typing import List, Optional


@dataclass
class App:
    """
    A dataclass to represent a single operating system distribution from the
    project.yaml file.

    Attributes:
        name (str): The name of the distribution (e.g., 'centos', 'debian').
        version (str): The version of the distribution (e.g., '7', '10').
        architecture (str): The system architecture (e.g., 'x86_64', 'arm64').
        package_manager (str): The type of package management, simplified to
                                'rpm' or 'deb'.
    """
    name: str
    supported_distro: List
    skipped_families: List
    supported_operating_systems: str
    package_manager_or_external: str

@dataclass
class FamilyDistro:
    """
    A dataclass to represent a single operating system distribution from the
    project.yaml file.

    Attributes:
        name (str): The name of the distribution (e.g., 'centos', 'debian').
        version (str): The version of the distribution (e.g., '7', '10').
        architecture (str): The system architecture (e.g., 'x86_64', 'arm64').
        package_manager (str): The type of package management, simplified to
                                'rpm' or 'deb'.
    """
    name: str
    image_family: str
    package_extension: str

def obtain_all_families() -> Optional[List[FamilyDistro]]:
    # Define the file path for the project.yaml file
    yaml_file = 'project.yaml'

    # Parse the YAML file
    distros = parse_project_yaml(yaml_file)
    if distros:
        print(f"Found {len(distros)} distributions:")
        print("---")
        for d in distros:
            print(f"  Name: {d.name}")
            print(f"  Family: {d.image_family}")
            print(f"  Package Manager: {d.package_extension}")
            print("---")

def parse_project_yaml(file_path: str) -> Optional[List[FamilyDistro]]:
    """
    Parses the project.yaml file and returns a list of Distro objects.

    Args:
        file_path (str): The path to the project.yaml file.

    Returns:
        Optional[List[Distro]]: A list of Distro objects if the file is
                                successfully parsed, otherwise None.
    """

    try:
        with open(file_path, 'r') as f:
            data = yaml.safe_load(f)
    except FileNotFoundError:
        print(f"Error: The file '{file_path}' was not found.")
        return None
    except yaml.YAMLError as e:
        print(f"Error parsing YAML file: {e}")
        return None

    # Access the nested 'os' data within each target
    targets_data = data.get('targets')

    if not isinstance(targets_data, dict):
        print("Error: 'targets' key not found or is not a dictionary in the YAML file.")
        return None

    distros_list = []
    for distro_name, distro_data in targets_data.items():
        package_extension = distro_data.get('package_extension', 'unknown')
        architectures = distro_data.get('architectures', {})
        x86_representative_architectures = architectures.get('x86_64', {}).get('test_distros', {}).get('representative',[])
        x86_exhaustive_architectures= architectures.get('x86_64', {}).get('test_distros', {}).get('exhaustive',[])
        arm_representative_architectures = architectures.get('aarch64', {}).get('test_distros', {}).get('representative',[])
        arm_exhaustive_architectures = architectures.get('aarch64', {}).get('test_distros', {}).get('exhaustive',[])
        families = x86_representative_architectures + x86_exhaustive_architectures + arm_representative_architectures+ arm_exhaustive_architectures
        name = distro_name
        for family in families:
        # Create a new Distro object and add it to the list
          distro_obj = FamilyDistro(
              name=name,
              image_family=family,
              package_extension=package_extension
          )
          distros_list.append(distro_obj)

    return distros_list

def parse_metadata(application_folder) -> List:
    yaml_file = 'metadata.yaml'

    # Parse the YAML file
    try:
        with open(os.path.join(application_folder,yaml_file), 'r') as f:
            data = yaml.safe_load(f)
    except FileNotFoundError:
        print(f"Error: The file '{file_path}' was not found.")
        return None
    except yaml.YAMLError as e:
        print(f"Error parsing YAML file: {e}")
        return None
    return data.get("platforms_to_skip",[]), data.get("supported_operating_systems", "unknown")
    # if metadata:
    #     print(f"Found {len(distros)} distributions:")
    #     print("---")
    #     for d in distros:
    #         print(f"  Name: {d.name}")
    #         print(f"  Family: {d.image_family}")
    #         print(f"  Package Manager: {d.package_extension}")
    #         print("---"))

def obtain_all_apps() -> Optional[List[App]]:
    applications_directory = os.path.abspath('./integration_test/third_party_apps_test/applications')
    entries = os.listdir(applications_directory)

    # Filter for directories only
    folders = [entry for entry in entries if os.path.isdir(os.path.join(applications_directory, entry))]
    apps = []
    if not folders:
        print(f"No folders found in '{applications_directory}'.")
    else:
        print(f"Folders in '{applications_directory}':")
        for folder in folders:
            # apps.append(App(folder,[],[],"package_manager"))
            print(f"- {folder}")
    if folders:
      print(f"Found {len(apps)} distributions:")
      print("---")
      for a in folders:
          skipped_families, supported_operating_systems = parse_metadata(os.path.join(applications_directory,a))
          app = App(a,[],skipped_families,supported_operating_systems,"package_manager")
          apps.append(app)

          print(f"  Name: {app.name}")
          print(f"  Family: {app.supported_distro}")
          print(f"  Supported Operating Systems: {app.supported_operating_systems}")
          print(f"  Skipped: {app.skipped_families}")
          print(f"  Package Manager or external: {app.package_manager_or_external}")
          print("---")
    return apps

if __name__ == '__main__':
    families = obtain_all_families()

    obtain_all_apps()


