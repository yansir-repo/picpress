// Package push implements the optional "compress and push" pipeline described
// in docs/push-upload.md.
//
// Invariants this package is built around:
//
//   - Write-only remote. The credentials can only PutObject and run multipart
//     uploads. Nothing here issues GET / HEAD / LIST; the S3 client does not
//     even expose such operations, so the class "depends on a read" cannot be
//     written.
//   - The journal is the only record of what was sent. Dedup, resume and the
//     batch manifests are all derived from it.
//   - Keys are a pure function of the plan: (prefix, dropped root names,
//     relative paths). Normalisation and collision resolution happen once, in
//     Plan, so the same drop always yields the same keys and dedup works.
//   - A manifest only lists results whose upload already completed: the
//     collector only ever receives finished results.
package push
