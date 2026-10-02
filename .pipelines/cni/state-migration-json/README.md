# JSON state migration lifecycle controls

This standalone Azure Pipelines source exercises the existing JSON state path on
representative managed Azure CNI overlay clusters. It is isolated from the
existing CNI pipelines: pull-request and continuous-integration triggers are
disabled, and a weekly `master` schedule is declared for use after the pipeline
is registered.

Each Linux and Windows control runs:

1. baseline state validation;
2. a CNS restart without a node reboot;
3. scale cycles with an active workload followed by another CNS restart;
4. a node reboot and post-reboot validation;
5. per-node CNS configuration, state, and assigned-IP JSON capture;
6. workload cleanup and unconditional cluster deletion.

The pipeline reuses the existing cluster creation, state validation, node
restart, and cluster deletion templates. It requires the same pool,
subscription, region, and service-connection variables as the existing CNI
release test. Registering the pipeline or provisioning shared infrastructure is
outside this scaffold.

Run local contracts with:

```bash
bash .pipelines/cni/state-migration-json/tests/run-contract-tests.sh
```
