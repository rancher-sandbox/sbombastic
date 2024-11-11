tilt_settings_file = "./tilt-settings.yaml"
settings = read_yaml(tilt_settings_file)

# Create the sbombastic namespace
# This is required since the helm() function doesn't support the create_namespace flag
load("ext://namespace", "namespace_create")
namespace_create("sbombastic")

controller_image = settings.get("controller").get("image")
storage_image = settings.get("storage").get("image")
worker_image = settings.get("worker").get("image")

yaml = helm(
    "./helm",
    name="sbombastic",
    namespace="sbombastic",
    set=[
        "controller.image.repository=" + controller_image,
        "storage.image.repository=" + storage_image,
        "worker.image.repository=" + worker_image,
        "controller.replicas=1",
        "storage.replicas=1",
        "worker.replicas=1"
    ]
)

k8s_yaml(yaml)

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
    ],
)

entrypoint = ["/controller"]
dockerfile = "./hack/Dockerfile.controller.tilt"

load("ext://restart_process", "docker_build_with_restart")
docker_build_with_restart(
    controller_image,
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
        "internal/admission",
        "internal/apiserver",
        "internal/storage",
        "pkg"
    ],
)

entrypoint = ["/storage"]
# We use a specific Dockerfile since tilt can't run on a scratch container.
dockerfile = "./hack/Dockerfile.storage.tilt"

load("ext://restart_process", "docker_build_with_restart")
docker_build_with_restart(
    storage_image,
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
    worker_image,
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
)
