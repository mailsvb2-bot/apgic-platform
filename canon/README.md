# Canon

The normative human-readable source is the supplied canonical artifact:

`APGIC_Единое_каноническое_ТЗ_исполняемый_канон_v7_FINAL.docx`

Expected SHA-256:

`37dff62a52d60e6011cabd0b5f91c9a50ef6a6981b80fcd0f149071554ed85f4`

The initial repository bootstrap commits the machine-readable execution companions:

- `requirements/registry.yaml`
- `requirements/coverage.json`

The DOCX binary is not duplicated in the first code bootstrap; its hash remains the immutable linkage to the approved source artifact. A repository copy may be added later without changing meaning.

The repository copy of the registry may advance implementation statuses and add implementation/test/evidence refs, but it must not silently alter requirement meaning. Meaning changes require Requirement Change / RFC plus synchronized coverage/dependency/acceptance updates.
