# SBOMscanner VEXHub

This directory contains all the files needed to setup a local VEX Hub repository.

All the information to set it up can be found here:

* https://github.com/aquasecurity/vex-repo-spec

* https://trivy.dev/v0.65/docs/advanced/self-hosting/#vex-hub

## Archive Generation

A VEX Hub repository must serve an archive with the content of the VEX files.

To create the archive run the following command:

```
tar -czvf main.tar.gz -C main/ .
```

This will create the `main.tar.gz` archive inside this directory.
