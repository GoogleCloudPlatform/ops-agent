// packer.pkr.hcl
variable "project_id" {
  type        = string
  description = "GCP Project ID"
}

variable "image_name" {
  type        = string
  description = "Name of the created GCE image"
}

variable "image_family" {
  type        = string
  description = "Image family for the created GCE image"
}

variable "source_image" {
  type        = string
  description = "The specific source GCE image name (e.g., ubuntu-2204-jammy-v20240115)"
}

variable "source_image_project" {
  type        = string
  description = "The specific source GCE image project (e.g., ubuntu-os-cloud)"
}


variable "gpu_driver_version" {
  type        = string
  default     = "535.161.01" // Pin specific NVIDIA driver version
  description = "Specific NVIDIA GPU driver version to install"
}

variable "cuda_version" {
  type        = string
  default     = "12.2.2" // Pin specific CUDA Toolkit version
  description = "Specific CUDA Toolkit version to install"
}

variable "zone" {
  type        = string
  default     = "us-central1-a"
  description = "GCP zone for the temporary build instance"
}

variable "build_id" {
  type        = string
  description = "Cloud Build ID for traceability"
  default     = "manual"
}

source "googlecompute" "gpu_image" {
  project_id          = var.project_id
  zone                = var.zone
  source_image        = var.source_image
  source_image_project_id = [var.source_image_project]
  image_name          = var.image_name
  image_family        = var.image_family
  ssh_username        = "packer"
  disk_size           = 50
  disk_type           = "pd-standard"
  machine_type        = "n1-standard-4" // Use a standard VM for building, no GPU needed here
  tags                = ["packer-build"]

  // *** IMPORTANT: Label the created image with its source image ***
  image_labels = {
    source-gce-image = "${var.source_image}"
    built-by         = "louhi"
    cloud-build-id   = "${var.build_id}"
  }
}

build {
  sources = ["source.googlecompute.gpu_image"]
  provisioner "shell" {
    script = "./scripts/${var.image_family}/setup_vm.sh"
    # Packer will pass these variables as PACKER_VAR_* env vars
    environment_vars = [
      "PACKER_VAR_project_id=${var.project_id}",
      "PACKER_VAR_gpu_driver_version=${var.gpu_driver_version}",
      "PACKER_VAR_cuda_version=${var.cuda_version}",
      "PACKER_VAR_build_id=${var.build_id}"
    ]
    # Expect a disconnect/reboot after GPU driver install
    expect_disconnect = true
    # Give some time for SSH to come back up
    timeout           = "240m"
  }

  // Provisioner 2: Handles the post-reboot part, ONLY for Debian 12.
  provisioner "shell" {
    script  = var.image_family == "debian-12" ? "./scripts/${var.image_family}/post_reboot.sh" : "./scripts/noop.sh"
    environment_vars = [
      "PACKER_VAR_project_id=${var.project_id}",
      "PACKER_VAR_gpu_driver_version=${var.gpu_driver_version}",
      "PACKER_VAR_cuda_version=${var.cuda_version}",
      "PACKER_VAR_build_id=${var.build_id}"
    ]
    # Wait for the reboot to be complete
    pause_before = "60s"
    expect_disconnect = false // No reboot expected in this second phase.
    timeout           = "240m"
  }
}
