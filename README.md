
# pod-time-measure-controller

## Overview

`pod-time-measure-controller` is a Kubernetes controller built using [Kubebuilder](https://book.kubebuilder.io/), designed to measure and record the startup time of pods in your cluster. The controller provides valuable insights into pod startup performance, helping operators and developers debug, optimize, and monitor workloads.

## Features

- Built with Kubebuilder for robust controller scaffolding and best practices.
- Watches for pod creation events in the cluster.
- Measures the time taken for pods to transition from Pending to Running state.
- Logs pod startup timings into a JSON file for easy analysis.
- JSON timing files are stored in a Persistent Volume (PV) via a Persistent Volume Claim (PVC) to ensure data persists across pod restarts and failures.
- Includes a `debug-pod` for accessing the PVC and reading the JSON timing data, since the main controller image is static and does not include tools like `tar`.
- Easily extendable for custom metrics or integrations.

## Architecture

- **Controller:** Watches pod events, measures startup time, and writes results to a JSON file mounted on a PVC.
- **Persistent Storage:** Uses a PV/PVC to store timing data, ensuring persistence and durability.
- **Debug Pod:** Provides shell access to the PVC for inspecting or extracting JSON timing files, overcoming limitations of the static controller image.

## Getting Started

### Prerequisites

- Kubernetes cluster (v1.20+ recommended)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- Go (for development/building)
- Docker (for building container images)

### Installation

1. **Clone the repository:**

   ```sh
   git clone https://github.com/karthikbhat19/pod-time-measure-controller.git
   cd pod-time-measure-controller
   ```

2. **Build the controller:**

   ```sh
   make manifests
   ```

3. **Apply PV/PVC and debug-pod manifests:**

   ```sh
   kubectl create ns pod-time-measure-controller-system
   kubectl apply -f pod-time-logger-pv.yaml
   kubectl apply -f pod-time-logger-pvc.yaml
   kubectl apply -f debug-pod.yaml
   ```

4. **Deploy to your cluster:**

   ```sh
   make deploy
   ```

### Usage

Once deployed, the controller will automatically start monitoring pod startup times and log them into a JSON file stored on the PVC. You can access the timing data by launching the debug pod:

```sh
kubectl exec -it debug-pod -- /bin/sh
cat /data/pod_startup_times.json
```

Replace `/data/pod_startup_times.json` with the actual mount path and filename as configured in your manifests.

### Notes

- The main controller image is static and does not include utilities like `tar` for extracting files. Use the debug pod for full shell access to the PVC.
- Timing data is persisted in the PVC and survives pod restarts and node failures.
- To build and push your image to dockerhub for your controller -  
`make docker-build IMG=<repo-name>/podtime-controller:latest`  
`make docker-push IMG=<repo-name>/podtime-controller:latest`

## Development

- Controller logic: [`internal/controller/podstartup_controller.go`](internal/controller/podstartup_controller.go)
- Main entry point: [`cmd/main.go`](cmd/main.go)
- CRDs and configuration: [`config/`](config/)
- PV/PVC and debug pod manifests: [`pod-time-logger-pv.yaml`, `pod-time-logger-pvc.yaml`, `debug-pod.yaml`]

### Running Locally

```sh
make run
```

### Testing

```sh
make test
```
