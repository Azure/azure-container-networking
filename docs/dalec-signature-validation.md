# Dalec signature validation

`verifyDalecSignatures` enables a pre-installation gate in ACN e2e and NPM
conformance. It defaults to `false`; ordinary image overrides are unchanged.
Only explicit, tagged acnpublic image parameters are selected. Empty inputs and
`__use-default__` are ignored; enabling the gate without an override fails.

The gate verifies the immutable image/index with Notation, then exports every
Linux platform with crane without executing the image. An authenticated index
covers the child manifest digests it references. Windows descriptors are excluded.
The platform filesystem is passed to a separately provisioned binary verifier.
Installation references retain the tag and gain the verified digest. No references
are published until all selected images and binaries pass.

## Provisioning prerequisites

The pipeline's trusted agent configuration must provide these variables:

| Variable | Required value |
| --- | --- |
| `DALEC_NOTATION_CONFIG` | Absolute directory containing approved `trustpolicy.json` and `truststore` certificates. |
| `DALEC_BINARY_VERIFIER` | Absolute executable path for the approved individual Linux binary signature verifier. |

Provision both independently of image contents and queue-time image parameters,
with appropriate access control. Do not take certificates or a verifier from the
image under test. The Notation policy must use `strict`, no verification overrides,
nonempty trust stores, and explicit signer identities (not `*`). The alternate
`trustpolicy.oci.json` is rejected. Registry authentication uses the existing build
validation service connection and a job-local Docker configuration.

The pipeline installs crane v0.21.3 and Notation v1.3.2 in a temporary directory.
Trust configuration is copied into a job-local Notation configuration. Temporary
credentials, exports, and tools are removed when the step exits.

**The individual Linux binary signing format and approved verifier are upstream
prerequisites, not implemented by this change.** There is intentionally no fallback
to RPM/package signatures, image signatures, payload checksums, or presence checks.
The gate fails if the approved binary verifier or trusted configuration is absent.
Enabling the caller before provisioning them will block its downstream tests.

## Binary verifier interface

The executable is invoked once per selected Linux platform:

```text
VERIFIER COMPONENT linux/ARCH[/VARIANT] REPOSITORY@sha256:DIGEST FILESYSTEM_TAR
```

The tar is the flattened image filesystem, not a package archive. The verifier
must safely inspect it without extracting untrusted paths into the host filesystem
or executing untrusted binaries. It must enforce a complete component-specific
product inventory, excluding OS utilities, and independently verify each product
binary against approved trust material. For CNI and IPAM, this includes DropGZ
and every embedded product executable, not just the launcher or its checksums.
Architecture-specific payloads must be inspected without deploying to host CNI
directories. Unknown formats, incomplete inventories, unsigned, modified, or
untrusted binaries must return nonzero.

On success it writes one JSON object to stdout; diagnostics go to stderr:

```json
{
  "verified": true,
  "inventoryComplete": true,
  "binaries": [
    {
      "path": "usr/bin/azure-npm",
      "sha256": "<64 lowercase hex characters>",
      "signer": "<verified signing identity>",
      "verified": true
    }
  ]
}
```

DropGZ payload entries additionally use `"embedded": true`. The gate requires the
expected main binary, successful evidence for every reported binary, an explicit
complete-inventory assertion, and embedded evidence for CNI/IPAM. The approved
verifier remains responsible for the exact payload inventory and cryptography;
structured output from an arbitrary executable is not proof of signing.

## Validation

```bash
bash -n hack/scripts/verify-dalec-signatures.sh
shellcheck hack/scripts/verify-dalec-signatures.sh
python3 hack/scripts/test_verify_dalec_signatures.py
python3 hack/scripts/test_notation_crypto.py /path/to/pinned/tools/bin
```

The tests use controlled tool doubles to validate selection, failure propagation,
platform coverage, binary evidence, and digest preservation. They do **not** prove
cryptographic correctness or that production images are signed.

The separate Notation crypto fixture uses the real pinned tools, a loopback registry
on a kernel-assigned port, and temporary test keys. A trusted COSE signature passes;
missing signatures and an untrusted signer fail. It removes its keys and stops the
registry on exit. This tests image-signature cryptography, not the unavailable
production binary signing contract.

Before enabling production callers, validate all eight mirrored components against
approved trust and binary verifiers. Then run ACN with all seven overrides plus a
companion NPM conformance run, and component-specific runs (CNI/CNS paired, IPAM
alone, ancillary components individually, and NPM separately). Missing mirrored
signatures and unavailable binary contracts remain failures/blockers, not passing
e2e results. Image mirroring and upstream signing are outside this gate's scope.
