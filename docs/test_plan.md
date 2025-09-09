# Test Plan

This plan ensures that SBOMBastic undergoes the following tests and has no unacceptable bugs before the official release.  


## Compatibility Testing
The following factors determine the testing combinations:
- SBOMBastic: Use the rc build. Ex: If releasing **v1.0.0** → test **v1.0.0-rc**
- Rancher Manager: Use the latest supported version.
- Distro: Use the latest versions of RKE2 and K3s supported by the Rancher Manager version.

For example, based on the current date, the following combinations are generated:
| SBOMBastic | Rancher Manager | Distro |
|:---:|:---:|:---:|
| v1.0.0-rc | v2.12.X | RKE2 v1.33 |  
| v1.0.0-rc | v2.12.X | K3S v1.33 | 

Test Items (Ensure consistent test results across all combinations):
- Deployment by helm:
    - Ensure sbombastic can be successfully deployed without error
    - Ensure all components are running
    - Ensure no panic or error message in controller/worker/storage logs after deployment
- Scan function:
    - Do manual scan and scheduled scan, and ensure:
      - the scan successfully generates and stores the expected output (image/sbom/vulnerabilityReport) for all image tags in the repository
      - the output can be correctly viewed without corruption
      - no panic or error message in controller/worker/storage logs after scanning
      - for scheduled scan, it will scan periodically for every scanInterval  
- Uninstall by helm:
    - Ensure sbombastic can be cleanly uninstalled


## Upgrade Testing
- Initial Official Release (v1.0.0):
    - Upgrade testing is not required.
- Subsequent Releases (ex: v1.0.1, v1.0.2, v2.0.1...etc):
    - Upgrade testing must cover upgrades from the two most recent prior versions (**if available**) to the latest version.
    - Examples:
      - If releasing **v1.0.1** → test **v1.0.0 → v1.0.1-rc**
      - If releasing **v1.0.2** → test **v1.0.0 → v1.0.2-rc** and **v1.0.1 → v1.0.2-rc**

Test Items (Post-Upgrade Verification):
- Ensure that the previous version's images/sbom/vulnerabilityReport are kept
- Ensure all components are running
- Ensure no panic or error message in controller/worker/storage logs
- Ensure that the system can perform a normal scan


## UI Automation Testing
Will file another doc for UI Automation Testing
