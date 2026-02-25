#!/usr/bin/env bash
# run-validation.sh — Run aksmigrate against the e2e cluster and validate outputs.
# Expects setup.sh to have been run first.
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BINARY="${REPO_ROOT}/aksmigrate"
OUTPUT_DIR="${SCRIPT_DIR}/output"
K8S_VERSION="${K8S_VERSION:-1.31}"
CLUSTER_NAME="${CLUSTER_NAME:-aksmigrate-e2e}"
RESOURCE_GROUP="${RESOURCE_GROUP:-aksmigrate-e2e-test}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0

pass() { echo -e "  ${GREEN}PASS${NC}: $*"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}FAIL${NC}: $*"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "${CYAN}[INFO]${NC}  $*"; }
section() { echo ""; echo -e "${YELLOW}=== $* ===${NC}"; }

###############################################################################
# Preflight
###############################################################################
[[ -x "${BINARY}" ]] || { echo "Binary not found at ${BINARY}. Run setup.sh first."; exit 1; }
kubectl cluster-info > /dev/null 2>&1 || { echo "Cannot connect to cluster."; exit 1; }
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

###############################################################################
# Test 1: Audit (JSON output)
###############################################################################
section "Test 1: Audit"
info "Running: aksmigrate audit --output json --k8s-version ${K8S_VERSION}"

# audit exits 1 on FAIL findings, which is expected
set +e
"${BINARY}" audit --output json --k8s-version "${K8S_VERSION}" > "${OUTPUT_DIR}/audit.json" 2>&1
AUDIT_EXIT=$?
set -e

info "Audit exit code: ${AUDIT_EXIT}"

# Verify we have findings
TOTAL_FINDINGS=$(jq '.findings | length' "${OUTPUT_DIR}/audit.json" 2>/dev/null || echo "0")
if [[ "${TOTAL_FINDINGS}" -gt 0 ]]; then
    pass "Audit produced ${TOTAL_FINDINGS} findings"
else
    fail "Audit produced no findings (expected multiple)"
fi

# --- Scenario 1: CILIUM-001 ipBlock catch-all ---
section "Scenario 1: CILIUM-001 (ipBlock catch-all)"
CILIUM001_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-001" and .namespace=="e2e-ipblock")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM001_HITS}" -gt 0 ]]; then
    pass "CILIUM-001 detected in e2e-ipblock namespace (${CILIUM001_HITS} finding(s))"
else
    fail "CILIUM-001 not detected in e2e-ipblock namespace"
fi

CILIUM001_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-001" and .namespace=="e2e-ipblock")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM001_SEVERITY}" == "FAIL" ]]; then
    pass "CILIUM-001 severity is FAIL"
else
    fail "CILIUM-001 severity is '${CILIUM001_SEVERITY}', expected 'FAIL'"
fi

CILIUM001_POLICY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-001" and .namespace=="e2e-ipblock")][0].policyName' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM001_POLICY}" == "egress-ipblock-catch-all" ]]; then
    pass "CILIUM-001 found on policy 'egress-ipblock-catch-all'"
else
    fail "CILIUM-001 found on policy '${CILIUM001_POLICY}', expected 'egress-ipblock-catch-all'"
fi

# --- Scenario 2: CILIUM-002 named ports ---
section "Scenario 2: CILIUM-002 (named ports)"
CILIUM002_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-002" and .namespace=="e2e-namedports")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM002_HITS}" -gt 0 ]]; then
    pass "CILIUM-002 detected in e2e-namedports namespace (${CILIUM002_HITS} finding(s))"
else
    fail "CILIUM-002 not detected in e2e-namedports namespace"
fi

# Check if conflicting mappings trigger FAIL
CILIUM002_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-002" and .namespace=="e2e-namedports")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM002_SEVERITY}" == "FAIL" ]]; then
    pass "CILIUM-002 severity is FAIL (conflicting port mappings detected)"
elif [[ "${CILIUM002_SEVERITY}" == "WARN" ]]; then
    pass "CILIUM-002 severity is WARN (named port detected, no conflicts)"
else
    fail "CILIUM-002 severity is '${CILIUM002_SEVERITY}', expected 'FAIL' or 'WARN'"
fi

# --- Scenario 3: CILIUM-003 endPort ---
section "Scenario 3: CILIUM-003 (endPort ranges)"
CILIUM003_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-003" and .namespace=="e2e-endport")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM003_HITS}" -gt 0 ]]; then
    pass "CILIUM-003 detected in e2e-endport namespace (${CILIUM003_HITS} finding(s))"
else
    fail "CILIUM-003 not detected in e2e-endport namespace"
fi

CILIUM003_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-003" and .namespace=="e2e-endport")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM003_SEVERITY}" == "FAIL" ]]; then
    pass "CILIUM-003 severity is FAIL (Cilium < 1.17)"
else
    fail "CILIUM-003 severity is '${CILIUM003_SEVERITY}', expected 'FAIL'"
fi

# --- Scenario 4: CILIUM-005 LB ingress ---
section "Scenario 4: CILIUM-005 (LB ingress enforcement)"
CILIUM005_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-005" and .namespace=="e2e-lb-ingress")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM005_HITS}" -gt 0 ]]; then
    pass "CILIUM-005 detected in e2e-lb-ingress namespace (${CILIUM005_HITS} finding(s))"
else
    fail "CILIUM-005 not detected in e2e-lb-ingress namespace"
fi

CILIUM005_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-005" and .namespace=="e2e-lb-ingress")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM005_SEVERITY}" == "FAIL" ]]; then
    pass "CILIUM-005 severity is FAIL"
else
    fail "CILIUM-005 severity is '${CILIUM005_SEVERITY}', expected 'FAIL'"
fi

# --- Scenario 5: CILIUM-004 host egress ---
section "Scenario 5: CILIUM-004 (implicit host egress)"
CILIUM004_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-004" and .namespace=="e2e-host-egress")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM004_HITS}" -gt 0 ]]; then
    pass "CILIUM-004 detected in e2e-host-egress namespace (${CILIUM004_HITS} finding(s))"
else
    fail "CILIUM-004 not detected in e2e-host-egress namespace"
fi

CILIUM004_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-004" and .namespace=="e2e-host-egress")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM004_SEVERITY}" == "WARN" ]]; then
    pass "CILIUM-004 severity is WARN"
else
    fail "CILIUM-004 severity is '${CILIUM004_SEVERITY}', expected 'WARN'"
fi

# --- Cross-cutting: CILIUM-007 kube-proxy removal ---
section "Cross-cutting: CILIUM-007 (kube-proxy removal)"
CILIUM007_HITS=$(jq '[.findings[] | select(.ruleId=="CILIUM-007")] | length' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM007_HITS}" -gt 0 ]]; then
    pass "CILIUM-007 (kube-proxy removal) info finding present"
else
    fail "CILIUM-007 (kube-proxy removal) not found in audit output"
fi

CILIUM007_SEVERITY=$(jq -r '[.findings[] | select(.ruleId=="CILIUM-007")][0].severity' "${OUTPUT_DIR}/audit.json")
if [[ "${CILIUM007_SEVERITY}" == "INFO" ]]; then
    pass "CILIUM-007 severity is INFO"
else
    fail "CILIUM-007 severity is '${CILIUM007_SEVERITY}', expected 'INFO'"
fi

###############################################################################
# Test 2: Translate
###############################################################################
section "Test 2: Translate"
TRANSLATE_DIR="${OUTPUT_DIR}/cilium-patches"
info "Running: aksmigrate translate --output-dir ${TRANSLATE_DIR} --k8s-version ${K8S_VERSION}"
"${BINARY}" translate --output-dir "${TRANSLATE_DIR}" --k8s-version "${K8S_VERSION}" 2>&1 | tee "${OUTPUT_DIR}/translate.log"

# Check patched directory exists
if [[ -d "${TRANSLATE_DIR}/patched" ]]; then
    PATCHED_COUNT=$(find "${TRANSLATE_DIR}/patched" -name "*.yaml" | wc -l)
    pass "Patched directory created with ${PATCHED_COUNT} file(s)"
else
    fail "Patched directory not created"
fi

# Check cilium directory exists
if [[ -d "${TRANSLATE_DIR}/cilium" ]]; then
    CILIUM_COUNT=$(find "${TRANSLATE_DIR}/cilium" -name "*.yaml" | wc -l)
    pass "Cilium directory created with ${CILIUM_COUNT} file(s)"
else
    fail "Cilium directory not created"
fi

# Scenario 1 translate: ipBlock patched policy should have namespaceSelector
PATCHED_IPBLOCK="${TRANSLATE_DIR}/patched/e2e-ipblock-egress-ipblock-catch-all.yaml"
if [[ -f "${PATCHED_IPBLOCK}" ]]; then
    pass "Patched ipBlock policy file exists"
    if grep -q "namespaceSelector" "${PATCHED_IPBLOCK}"; then
        pass "Patched ipBlock policy contains namespaceSelector"
    else
        fail "Patched ipBlock policy missing namespaceSelector"
    fi
else
    fail "Patched ipBlock policy file not found: ${PATCHED_IPBLOCK}"
fi

# Scenario 5 translate: host egress CiliumNetworkPolicy
HOST_EGRESS_CNP="${TRANSLATE_DIR}/cilium/e2e-host-egress-allow-host-egress.yaml"
if [[ -f "${HOST_EGRESS_CNP}" ]]; then
    pass "Host egress CiliumNetworkPolicy file exists"
    if grep -q "toEntities" "${HOST_EGRESS_CNP}" && grep -q "host" "${HOST_EGRESS_CNP}"; then
        pass "Host egress CNP contains toEntities: [host]"
    else
        fail "Host egress CNP missing toEntities host"
    fi
else
    fail "Host egress CiliumNetworkPolicy not found: ${HOST_EGRESS_CNP}"
fi

# Scenario 4 translate: LB ingress CiliumNetworkPolicy
LB_INGRESS_CNP="${TRANSLATE_DIR}/cilium/e2e-lb-ingress-allow-lb-ingress-web-lb.yaml"
if [[ -f "${LB_INGRESS_CNP}" ]]; then
    pass "LB ingress CiliumNetworkPolicy file exists"
    if grep -q "fromEntities" "${LB_INGRESS_CNP}" && grep -q "world" "${LB_INGRESS_CNP}"; then
        pass "LB ingress CNP contains fromEntities: [world]"
    else
        fail "LB ingress CNP missing fromEntities world"
    fi
else
    fail "LB ingress CiliumNetworkPolicy not found: ${LB_INGRESS_CNP}"
fi

###############################################################################
# Test 3: Conntest snapshot
###############################################################################
section "Test 3: Conntest Snapshot"
SNAPSHOT_FILE="${OUTPUT_DIR}/pre-migration-snapshot.json"
info "Running: aksmigrate conntest snapshot --phase pre-migration --output ${SNAPSHOT_FILE}"
"${BINARY}" conntest snapshot --phase pre-migration --output "${SNAPSHOT_FILE}" 2>&1 | tee "${OUTPUT_DIR}/conntest.log"

if [[ -f "${SNAPSHOT_FILE}" ]]; then
    pass "Snapshot file created"

    RESULT_COUNT=$(jq '.results | length' "${SNAPSHOT_FILE}" 2>/dev/null || echo "0")
    if [[ "${RESULT_COUNT}" -gt 0 ]]; then
        pass "Snapshot has ${RESULT_COUNT} connectivity results"
    else
        fail "Snapshot has no connectivity results"
    fi

    PHASE=$(jq -r '.phase' "${SNAPSHOT_FILE}" 2>/dev/null || echo "")
    if [[ "${PHASE}" == "pre-migration" ]]; then
        pass "Snapshot phase is 'pre-migration'"
    else
        fail "Snapshot phase is '${PHASE}', expected 'pre-migration'"
    fi

    TIMESTAMP=$(jq -r '.timestamp' "${SNAPSHOT_FILE}" 2>/dev/null || echo "")
    if [[ -n "${TIMESTAMP}" && "${TIMESTAMP}" != "null" ]]; then
        pass "Snapshot has timestamp: ${TIMESTAMP}"
    else
        fail "Snapshot missing timestamp"
    fi
else
    fail "Snapshot file not created: ${SNAPSHOT_FILE}"
fi

###############################################################################
# Test 4: Migrate --dry-run
###############################################################################
section "Test 4: Migrate --dry-run"
MIGRATE_DIR="${OUTPUT_DIR}/migration-output"
info "Running: aksmigrate migrate --cluster-name ${CLUSTER_NAME} --resource-group ${RESOURCE_GROUP} --output-dir ${MIGRATE_DIR} --k8s-version ${K8S_VERSION} --dry-run --skip-snapshot"
"${BINARY}" migrate \
    --cluster-name "${CLUSTER_NAME}" \
    --resource-group "${RESOURCE_GROUP}" \
    --output-dir "${MIGRATE_DIR}" \
    --k8s-version "${K8S_VERSION}" \
    --dry-run \
    --skip-snapshot 2>&1 | tee "${OUTPUT_DIR}/migrate-dryrun.log"

# Validate 7-step markers
for step in 1 2 3 4 5 6 7; do
    if grep -q "\[${step}/7\]" "${OUTPUT_DIR}/migrate-dryrun.log"; then
        pass "Migrate dry-run step [${step}/7] marker present"
    else
        fail "Migrate dry-run step [${step}/7] marker missing"
    fi
done

# Validate patches directory created
if [[ -d "${MIGRATE_DIR}/patches" ]]; then
    MIGRATE_PATCH_COUNT=$(find "${MIGRATE_DIR}/patches" -name "*.yaml" 2>/dev/null | wc -l)
    pass "Migration patches directory created (${MIGRATE_PATCH_COUNT} files)"
else
    fail "Migration patches directory not created"
fi

# Validate final report
if grep -q "Migration Report" "${OUTPUT_DIR}/migrate-dryrun.log"; then
    pass "Migration final report present"
else
    fail "Migration final report missing"
fi

if grep -q "DRY RUN" "${OUTPUT_DIR}/migrate-dryrun.log"; then
    pass "Dry run mode indicated in report"
else
    fail "Dry run mode not indicated in report"
fi

###############################################################################
# Summary
###############################################################################
echo ""
echo "============================================="
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo -e "Results: ${GREEN}${PASS_COUNT} PASS${NC} / ${RED}${FAIL_COUNT} FAIL${NC} / ${TOTAL} total"
echo "============================================="
echo "Output files in: ${OUTPUT_DIR}/"
echo ""

if [[ "${FAIL_COUNT}" -gt 0 ]]; then
    echo -e "${RED}VALIDATION FAILED: ${FAIL_COUNT} check(s) did not pass.${NC}"
    exit 1
else
    echo -e "${GREEN}ALL CHECKS PASSED!${NC}"
    exit 0
fi
