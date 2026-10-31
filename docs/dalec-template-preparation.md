# Prepare Dalec templates before tagging

The Create Release Tag workflow can queue the private ACN template preparation
pipeline in Azure DevOps project `One`, before entering the existing
`tag-approval` job.

The request uses the validated proposed tag and full `target_sha`. Preparation
must return a matching pipeline result and a created/reused Dalec PR, or a
verified unchanged recipe. Tagging does **not** wait for the PR to merge.
Dalec separately requires a reviewed recipe before generating/building a release.

## Enable after private infrastructure is ready

Set these repository variables:

| Variable | Meaning |
|---|---|
| `DALEC_SYNC_PIPELINE_ID` | Registered `DalecTemplateSync.yml` pipeline ID in `msazure/One` |
| `DALEC_SYNC_CLIENT_ID` | Federated workload identity with queue/read access to that pipeline and its artifacts |
| `AZURE_TENANT_ID` | Tenant for the workload identity |
| `DALEC_TEMPLATE_SYNC_ENABLED` | Set to `true` only after the private pipeline, approvals and Dalec release-readiness checks are operational |

Authorize the GitHub OIDC subject for the release workflow's permitted branch.
This identity needs no Dalec repository write permission and no tag permission.
The private pipeline owns its separately scoped PR-publishing identity.

While the feature is disabled, the existing tag flow remains unchanged. Once
enabled, queue failure, timeout, failed preparation, missing artifact or mismatched
tag/SHA/run prevents the tag job from running. Pipeline results cannot silently
classify an onboarded image as not applicable.

The tag-approval environment, annotated tag message, deployment recording and
remote-tag checks remain in place. Scheduled root and component releases already
dispatch the same workflow and inherit the preparation step.

The workflow explicitly queues preparation; it does not depend on tag-push
events. Retries reuse the release's preparation PR. If the tag already exists,
use the private pipeline's manual retry rather than moving that tag.

```sh
python3 -m unittest discover -s .github/scripts -p test_prepare_dalec.py
```
