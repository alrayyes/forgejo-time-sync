"""End-to-end: real Forgejo container, real Kimai container, no mocks on
either side. Session-scoped because each container takes real wall-clock
time to boot (Kimai in particular: MySQL + migrations + the wizard dance).
"""

import httpx
import pytest
from forgejo_fixture import start_forgejo
from kimai_fixture import start_kimai

from forgejo_time_sync.sync import ForgejoClient, KimaiClient, sync_repo_times

OWNER = "testowner"
REPO = "test-repo"


@pytest.fixture(scope="session")
def forgejo():
    instance = start_forgejo()
    yield instance
    instance.container.stop()


@pytest.fixture(scope="session")
def kimai():
    instance = start_kimai()
    yield instance
    instance.kimai_container.stop()
    instance.mysql_container.stop()
    instance.network.remove()


@pytest.fixture(scope="session")
def forgejo_client(forgejo):
    return ForgejoClient(base_url=forgejo.base_url, token=forgejo.token)


@pytest.fixture(scope="session")
def kimai_client(kimai):
    return KimaiClient(base_url=kimai.base_url, user=kimai.user, token=kimai.token)


@pytest.fixture(scope="session")
def kimai_project(kimai):
    auth = {"X-AUTH-USER": kimai.user, "X-AUTH-TOKEN": kimai.token}
    customer = httpx.post(
        f"{kimai.base_url}/api/customers",
        headers=auth,
        json={"name": f"{OWNER}-{REPO}", "visible": True},
    ).raise_for_status().json()
    project = httpx.post(
        f"{kimai.base_url}/api/projects",
        headers=auth,
        json={"name": REPO, "customer": customer["id"], "visible": True},
    ).raise_for_status().json()
    activity = httpx.post(
        f"{kimai.base_url}/api/activities",
        headers=auth,
        json={"name": "forgejo-sync", "visible": True},
    ).raise_for_status().json()
    return project["id"], activity["id"]


@pytest.fixture(scope="session")
def forgejo_issue(forgejo):
    auth = {"Authorization": f"token {forgejo.token}"}
    httpx.post(
        f"{forgejo.base_url}/api/v1/user/repos",
        headers=auth,
        json={"name": REPO, "auto_init": False},
    ).raise_for_status()
    issue = httpx.post(
        f"{forgejo.base_url}/api/v1/repos/{OWNER}/{REPO}/issues",
        headers=auth,
        json={"title": "Test issue for time tracking"},
    ).raise_for_status().json()
    httpx.post(
        f"{forgejo.base_url}/api/v1/repos/{OWNER}/{REPO}/issues/{issue['number']}/times",
        headers=auth,
        json={"time": 5400},
    ).raise_for_status()
    return issue


def test_sync_pushes_forgejo_time_into_kimai(forgejo_client, kimai_client, kimai_project, forgejo_issue):
    project_id, activity_id = kimai_project

    created = sync_repo_times(
        forgejo=forgejo_client,
        kimai=kimai_client,
        owner=OWNER,
        repo=REPO,
        kimai_project=project_id,
        kimai_activity=activity_id,
    )

    assert len(created) == 1
    assert created[0]["duration"] == 5400
    assert f"issue:{OWNER}/{REPO}#{forgejo_issue['number']}" in created[0]["description"]

    timesheets = kimai_client.list_timesheets()
    assert len(timesheets) == 1


def test_sync_is_idempotent_on_rerun(forgejo_client, kimai_client, kimai_project, forgejo_issue):
    project_id, activity_id = kimai_project

    created = sync_repo_times(
        forgejo=forgejo_client,
        kimai=kimai_client,
        owner=OWNER,
        repo=REPO,
        kimai_project=project_id,
        kimai_activity=activity_id,
    )

    assert created == []
    assert len(kimai_client.list_timesheets()) == 1
