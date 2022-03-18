# Kusto line used to make the table schema
# .alter table acncodecovtable (ProjectName:string,ProjectId:string,RepositoryName:string,RepositoryId:string,Timestamp:datetime,SourceBranchName:string,BuildReason:string,BuildId:string,BuildLink:string,PullRequestId:string,PullRequestLink:string,PullRequestSubmitter:string,GithubPRId:string,Library:string,TotalLines:int,LinesCovered:int,CoveragePercentage:real,BaslelineTotalLines:int,BaselineLinesCovered:int,BaselineCoveragePercentage:real,DiffCoverage:real)

import os
from tabulate import tabulate
import pandas as pd
import pprint
import time
from datetime import datetime
from azure.kusto.ingest import (
    QueuedIngestClient,
    IngestionProperties,
    ReportLevel,
)
from azure.kusto.data import KustoConnectionStringBuilder
from azure.kusto.ingest.status import KustoIngestStatusQueues

def get_coverage_stats():
    project_name = os.getenv("SYSTEM_TEAMPROJECT")
    project_id = os.getenv("SYSTEM_TEAMPROJECTID")
    repository_name = os.getenv("BUILD_REPOSITORY_NAME")
    repository_id = os.getenv("BUILD_REPOSITORY_ID")
    timestamp = datetime.strptime(os.getenv("SYSTEM_PIPELINESTARTTIME"), "%Y-%m-%d %H:%M:%S+00:00")
    build_reason = os.getenv("BUILD_REASON")
    source_branch_name = os.getenv("BUILD_SOURCEBRANCHNAME")
    build_id = os.getenv("BUILD_BUILDID")
    build_link = "https://dev.azure.com/msazure/One/_build/results?buildId=" + str(build_id)
    pr_id = os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTID")
    pr_link = get_pr_url()
    pr_submitter = get_pr_submitter()
    github_pr_id = get_github_pr_id()
    library = os.getenv("BUILD_DEFINITIONNAME")
    total_lines = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_ELEMENTS_TOTAL")
    lines_covered = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_ELEMENTS_COVERED")
    coverage_percentage = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_PERCENTAGE_COVERED")
    baseline_total_lines = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_ELEMENTS_TOTAL")
    baseline_lines_covered = os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_ELEMENTS_COVERED")
    baseline_coverage_percentage= os.getenv("BUILDQUALITYCHECKS_CODECOVERAGEPOLICY_BASELINE_PERCENTAGE_COVERED")
    diff_coverage = float(coverage_percentage) - float(baseline_coverage_percentage)
    return pd.DataFrame({'ProjectName':[project_name], 'ProjectId':[project_id], 'RepositoryName':[repository_name], 'RepositoryId':[repository_id], 'Timestamp':[timestamp], 'SourceBranchName': [source_branch_name], 'BuildReason':[build_reason], 'BuildId':[build_id], 'BuildLink':[build_link], 'PullRequestId':[pr_id], 'PullRequestLink':[pr_link], 'PullRequestSubmitter':[pr_submitter], 'GithubPRId':[github_pr_id],  'Library':[library], 'totalLines':[total_lines], 'linesCovered':[lines_covered], 'CoveragePercentage':[coverage_percentage], 'BaslelineTotalLines':[baseline_total_lines], 'BaselineLinesCovered':[baseline_lines_covered], 'BaselineCoveragePercentage':[baseline_coverage_percentage], 'DiffCoverage':[diff_coverage]})

def get_github_pr_id():
    repo_url = os.getenv("BUILD_REPOSITORY_URI")
    if "github" in repo_url:
        pr_number = os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER")
        if pr_number is None:
            return ""
        return pr_number
    return ""

def get_pr_url():
    repo_url = os.getenv("BUILD_REPOSITORY_URI")
    if "github" in repo_url:
        pr_number = os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER")
        if pr_number is None:
            return ""
        return repo_url + "/pull/" + os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER")

    pr_id = os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTID")
    if pr_id is None:
        return ""

    return repo_url + "/pullrequest/" + os.getenv("SYSTEM_PULLREQUEST_PULLREQUESTID")

def get_pr_submitter():
    repo_url = os.getenv("BUILD_REPOSITORY_URI")
    if "github" in repo_url:
        author = os.getenv("BUILD_SOURCEVERSIONAUTHOR")
        if author is None:
            return ""
        return author
    pr_submitter_email = os.getenv("BUILD_REQUESTEDFOREMAIL")
    pr_submitter = pr_submitter_email.split('@')[0]
    return pr_submitter

def publish_metrics_kusto(df):
    cluster_ingest_url = os.getenv("KUSTO_INGEST_CLUSTER_URL")
    db_name = os.getenv("KUSTO_DB_NAME")
    table_name = os.getenv("KUSTO_TABLE_NAME")
    client_id = os.getenv("KUSTO_SERVICE_PRINCIPAL_ID")
    client_secret = os.getenv("KUSTO_SERVICE_PRINCIPAL_PASSWORD")
    tenant_id = os.getenv("TENANT_ID")

    print(tabulate(df, headers = 'keys', tablefmt = 'psql'))

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
