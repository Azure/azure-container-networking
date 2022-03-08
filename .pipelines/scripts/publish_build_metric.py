import os
import requests
import pandas as pd
import argparse
import pprint
import time
import datetime
import base64
from azure.kusto.ingest import (
    QueuedIngestClient,
    IngestionProperties,
    ReportLevel,
)
from azure.kusto.data import KustoConnectionStringBuilder
from azure.kusto.ingest.status import KustoIngestStatusQueues

def get_coverage_stats():
    repository_id = os.getenv("BUILD_REPOSITORY_ID")
    repository_name = os.getenv("BUILD_REPOSITORY_NAME")
    library = os.getenv("BUILD_DEFINITIONNAME")
    total_lines = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_ELEMENTS_TOTAL")
    lines_covered = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_ELEMENTS_COVERED")
    coverage_percentage = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_PERCENTAGE_COVERED")
    baseline_lines_covered = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_ELEMENTS_COVERED")
    baseline_total_lines = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_ELEMENTS_TOTAL")
    baseline_coverage_percentage= os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_PERCENTAGE_COVERED")
    diff_coverage = float(coverage_percentage) - float(baseline_coverage_percentage)
    return pd.DataFrame({'RepositoryName': [repository_name], 'RepositoryId' : [repository_id], 'Day' : [datetime.datetime.strftime(datetime.datetime.now(), "%Y-%m-%dT%H:%M:%SZ")], 'Library':[library], 'CoveragePercentage':[coverage_percentage], 'totalLines': [total_lines], 'linesCovered': [lines_covered], 'BaselineCoveragePercentage' : [baseline_coverage_percentage], 'BaslelineTotalLines' : [baseline_total_lines], 'BaselineLinesCovered' : [baseline_lines_covered], 'DiffCoverage': [diff_coverage] })

def publish_metrics_kusto(df):
    cluster_ingest_url = os.getenv("KUSTO_INGEST_CLUSTER_URL")
    db_name = os.getenv("KUSTO_DB_NAME")
    table_name = os.getenv("KUSTO_TABLE_NAME")
    client_id = os.getenv("KUSTO_SERVICE_PRINCIPAL_ID")
    client_secret = os.getenv("KUSTO_SERVICE_PRINCIPAL_PASSWORD")
    tenant_id = os.getenv("TENANT_ID")

    print(df.head())

    if df.empty:
        return

    kcsb = KustoConnectionStringBuilder.with_aad_application_key_authentication(cluster_ingest_url, client_id, client_secret, tenant_id)

    client = QueuedIngestClient(kcsb)

    ingestion_props = IngestionProperties(
        database=db_name,
        table=table_name,
        report_level=ReportLevel.FailuresAndSuccesses,
    )

    client.ingest_from_dataframe(df, ingestion_properties=ingestion_props)

    qs = KustoIngestStatusQueues(client)
    MAX_BACKOFF = 180
    backoff = 1
    while True:
        ################### NOTICE ####################
        # in order to get success status updates,
        # make sure ingestion properties set the
        # reportLevel=ReportLevel.FailuresAndSuccesses.
        if qs.success.is_empty() and qs.failure.is_empty():
            time.sleep(backoff)
            backoff = min(backoff * 2, MAX_BACKOFF)
            print("No new messages. backing off for {} seconds".format(backoff))
            continue

        backoff = 1

        success_messages = qs.success.pop(10)
        failure_messages = qs.failure.pop(10)

        pprint.pprint("SUCCESS_LIST : {}".format(success_messages))
        pprint.pprint("FAILURES_LIST : {}".format(failure_messages))
        break

if __name__ == "__main__":
    df = get_coverage_stats()
    publish_metrics_kusto(df)
    print(df)