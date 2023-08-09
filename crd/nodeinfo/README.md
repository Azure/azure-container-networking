# NodeInfo CRDs

This CRD is added to enable SWIFT multitenancy – which will be watched and managed by the MT-DNC-RC controller.


NodeInfo objects are created by Swift CNS as part of the node registration flow, and is used to pass any metadata from the VM needed by DNC / DNC-RC. E.g.: the vmUniqueID for use in PubSub queries / checks.
