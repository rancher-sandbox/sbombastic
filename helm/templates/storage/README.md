# SBOMbastic Storage

The `storage` Helm chart installs the SBOMbastic storage deployment, which should be installed alongside the SBOMbastic controller and worker components.

The storage component uses SQLite as its database backend. To ensure data persistence, it requires a PersistentVolumeClaim (PVC) backed by a PersistentVolume (PV). Users need to either:

1. Have a StorageClass configured that can dynamically provision PVs, or
2. Manually create a PV that matches the PVC requirements