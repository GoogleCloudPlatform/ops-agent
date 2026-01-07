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

  // Provisioner 1: Most distros only need one step
  provisioner "shell" {
    script = "/workspace/louhi_ws/cloudbuild/gpu-image-builder/scripts/${var.image_family}/setup_vm.sh"
    expect_disconnect = true  // Expect a disconnect/reboot after GPU driver install
    timeout           = "240m"
  }

  // Provisioner 2: Handles the post-reboot part, ONLY for Debian 12.
  provisioner "shell" {
    script  = var.image_family == "debian-12" ? "/workspace/louhi_ws/cloudbuild/gpu-image-builder/scripts/${var.image_family}/post_reboot.sh" : "/workspace/louhi_ws/cloudbuild/gpu-image-builder/scripts/noop.sh"
    pause_before = "60s" // Wait for the reboot to be complete
    expect_disconnect = false // No reboot expected in this second phase.
    timeout           = "240m"
  }
}
