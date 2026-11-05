# Vendored schema `frostyard.problem/1`

Byte copies from frostyard/core at commit
`f1b62e0692adc1da6050c6f4733814f06b7476c6`:

- `v1.json` — `docs/specs/schemas/problem-report/v1.json`
  (sha256 `6c053ee3a9b4fd399d1d011b2be833dc6b17f25e727161fc576b0ce64cc14c3a`)
- `examples/*.json` — `docs/specs/schemas/problem-report/examples/`, 1 valid
  document for each kind

`TestVendoredSchemaIsPinned` pins the sha256 of `v1.json` and the core
commit. An accepted schema file never changes (core spec rule 5); a new
schema version is a new file and a new pin.

`../snosi/` holds real output of the snosi collector `snosi-health`
(snosi ADR-0029, format 1): `problems.json` and `details.json`.
