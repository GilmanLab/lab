# Bootstrap PXE VM (temporary)

This folder contains the **temporary bootstrap PXE VM** used to bring the lab mini-PC online *without* the default provisioning agent.

Primary entrypoints:

- `packer/bootstrap.pkr.hcl`: Packer template that builds a **RAW** Ubuntu VM image.
- `files/`: dnsmasq + nginx + iPXE configuration files.
- `config/`: Talos machineconfig files served to the mini-PC.

See `DESIGN.md` for the full design + runbook.

### Artifact note

`build.sh` now **leaves the output uncompressed** by default (`bootstrap-pxe.raw`) so integration tests can run without immediately undoing compression. Use `COMPRESS=true` (or `UPLOAD=true`) when you’re ready to publish.


