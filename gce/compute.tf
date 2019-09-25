variable "zone" {
  default = "us-west1-c" // We're going to need it in several places in this config
}

variable "region" {
  default = "us-west1"
}

variable "project" {
  default = "sample-project"
}

provider "google" {
  credentials = "${file("account.json")}"
  project     = "${var.project}"
  region      = "${var.region}"
}

resource "google_compute_instance" "test" {
  count        = 2                            // Adjust as desired
  name         = "tf-test-${count.index + 1}" // yields "test1", "test2", etc. It's also the machine's name and hostname
  machine_type = "f1-micro"                   // smallest (CPU &amp; RAM) available instance
  zone         = "${var.zone}"                // yields "europe-west1-d" as setup previously. Places your VM in Europe

  boot_disk {
    initialize_params {
      image = "ubuntu-1604-xenial-v20180424" // the operative system (and Linux flavour) that your machine will run
    }
  }

  network_interface {
    network = "default"

    access_config {
      // Ephemeral IP - leaving this block empty will generate a new external IP and assign it to the machine
    }
  }
}
