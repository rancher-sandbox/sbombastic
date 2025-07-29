tilt_settings_file = "./tilt-settings.yaml"
settings = read_yaml(tilt_settings_file)

update_settings(k8s_upsert_timeout_secs=300)

# Setup a development registry so we can push images to it
# and use them to test the scanner.
k8s_yaml("./hack/registry.yaml")

k8s_resource(
    "dev-registry",
    port_forwards=5000,
)


# Install cert-manager
#
# Note: We are not using the tilt cert-manager extension, since it creates a namespace to test cert-manager,
# which takes a long time to delete when running `tilt down`.
# We Install the cert-manager CRDs separately, so we are sure they will be avalable before the sbombastic Helm chart is installed.
cert_manager_version = "v1.17.2"
local_resource(
    "cert-manager-crds",
    cmd="kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/{}/cert-manager.crds.yaml".format(
        cert_manager_version
    ),
)

load("ext://helm_resource", "helm_resource", "helm_repo")
helm_repo("jetstack", "https://charts.jetstack.io")
helm_resource(
    "cert-manager",
    "jetstack/cert-manager",
    namespace="cert-manager",
    flags=[
        "--version",
        cert_manager_version,
        "--create-namespace",
        "--set",
        "installCRDs=false",
    ],
    resource_deps=[
        "jetstack",
        "cert-manager-crds",
    ],
)


# Create the sbombastic namespace
# This is required since the helm() function doesn't support the create_namespace flag
load("ext://namespace", "namespace_create")
namespace_create("sbombastic")

helm_repo("cnpg-repo", "https://cloudnative-pg.github.io/charts")
helm_resource(
    "cnpg",
    "cnpg/cloudnative-pg",
    namespace="sbombastic",
    flags=[
        "--set",
        "config.clusterWide=false",
    ],
    resource_deps=[
        "cnpg-repo",
    ],
)

registry = settings.get("registry")
controller_image = settings.get("controller").get("image")
storage_image = settings.get("storage").get("image")
worker_image = settings.get("worker").get("image")


controller_image_dep = registry + "/" + controller_image
storage_image_dep = registry + "/" + storage_image
worker_image_dep = registry + "/" + worker_image

helm_resource(
    name         = "sbombastic",
    chart        = "./charts/sbombastic",
    release_name = "sbombastic",
    namespace    = "sbombastic",

    flags = [
        "--set=global.cattle.systemDefaultRegistry=", # ensure there is no double registry, like ghcr.io/ghcr.io/sbombastic/controller
        "--set=controller.image.repository=" + controller_image,
        "--set=worker.image.repository="     + worker_image,
        "--set=storage.image.repository="    + storage_image,
        "--set=controller.replicas=1",
        "--set=worker.replicas=1",
        "--set=storage.replicas=1",
        "--set=security.harden_deployment=false",   # if we enable this, the live reload will not work
        "--set=cnpg.enabled=true",
        "--set=controller.logLevel=debug",
        #  TODO: uncomment this, when the log parser in storage is implemented
        # "--set=storage.logLevel=debug",
        "--set=worker.logLevel=debug",
        "--set=storage.database.enableBuiltInCnpg=true",
    ],

    # # Tell Tilt which locally-built images map to which values
    image_deps = [
        controller_image_dep,
        worker_image_dep,
        storage_image_dep,
    ],
    image_keys = [
        ("controller.image.repository", "controller.image.tag"),
        ("worker.image.repository",     "worker.image.tag"),
        ("storage.image.repository",    "storage.image.tag"),
    ],

    # Wait for cert-manager, cnpg first
    resource_deps = [
        "cert-manager",          # if you installed cert-manager above
        "cnpg",                  # ensure the webhook is ready, so we then deploy the cluster
    ],
)

# Hot reloading containers
local_resource(
    "controller_tilt",
    "make controller",
    deps=[
        "go.mod",
        "go.sum",
        "cmd/controller",
        "api",
        "internal/controller",
        "internal/messaging",
    ],
)

entrypoint = ["/controller"]
dockerfile = "./hack/Dockerfile.controller.tilt"

load("ext://restart_process", "docker_build_with_restart")
docker_build_with_restart(
    registry + "/" + controller_image,
    ".",
    dockerfile=dockerfile,
    entrypoint=entrypoint,
    # `only` here is important, otherwise, the container will get updated
    # on _any_ file change.
    only=[
        "./bin/controller",
    ],
    live_update=[
        sync("./bin/controller", "/controller"),
    ],
)

local_resource(
    "storage_tilt",
    "make storage",
    deps=[
        "go.mod",
        "go.sum",
        "cmd/storage",
        "api",
        "internal/apiserver",
        "internal/storage",
        "pkg",
    ],
)

entrypoint = ["/storage"]
# We use a specific Dockerfile since tilt can't run on a scratch container.
dockerfile = "./hack/Dockerfile.storage.tilt"

load("ext://restart_process", "docker_build_with_restart")
docker_build_with_restart(
    registry + "/" + storage_image,
    ".",
    dockerfile=dockerfile,
    entrypoint=entrypoint,
    # `only` here is important, otherwise, the container will get updated
    # on _any_ file change.
    only=[
        "./bin/storage",
    ],
    live_update=[
        sync("./bin/storage", "/storage"),
    ],
)


local_resource(
    "worker_tilt",
    "make worker",
    deps=[
        "go.mod",
        "go.sum",
        "cmd/worker",
        "api",
        "internal/messaging",
        "internal/handlers",
    ],
)

entrypoint = ["/worker"]
# We use a specific Dockerfile since tilt can't run on a scratch container.
dockerfile = "./hack/Dockerfile.worker.tilt"

load("ext://restart_process", "docker_build_with_restart")
docker_build_with_restart(
    registry + "/" + worker_image,
    ".",
    dockerfile=dockerfile,
    entrypoint=entrypoint,
    # `only` here is important, otherwise, the container will get updated
    # on _any_ file change.
    only=[
        "./bin/worker",
    ],
    live_update=[
        sync("./bin/worker", "/worker"),
    ],
    # We need to change the default restart file, since the /tmp directory is an emptyDir volumeMount in this Pod
    # and tilt doesn't seem to be able to work with it.
    restart_file="/.restart-proc",
)
