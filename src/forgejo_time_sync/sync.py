"""Pulls Forgejo time entries into Kimai timesheets, idempotently."""

from dataclasses import dataclass

import httpx

DESCRIPTION_PREFIX = "forgejo-time-entry:"


@dataclass(frozen=True)
class ForgejoClient:
    base_url: str
    token: str

    def _get(self, path: str, **params) -> httpx.Response:
        response = httpx.get(
            f"{self.base_url}{path}",
            headers={"Authorization": f"token {self.token}"},
            params=params,
        )
        response.raise_for_status()
        return response

    def list_repo_times(self, owner: str, repo: str) -> list[dict]:
        return self._get(f"/api/v1/repos/{owner}/{repo}/times").json()


@dataclass(frozen=True)
class KimaiClient:
    base_url: str
    user: str
    token: str

    def _headers(self) -> dict:
        return {"X-AUTH-USER": self.user, "X-AUTH-TOKEN": self.token}

    def list_timesheets(self) -> list[dict]:
        response = httpx.get(f"{self.base_url}/api/timesheets", headers=self._headers())
        response.raise_for_status()
        return response.json()

    def create_timesheet(
        self, *, project: int, activity: int, begin: str, end: str, description: str
    ) -> dict:
        response = httpx.post(
            f"{self.base_url}/api/timesheets",
            headers=self._headers(),
            json={
                "project": project,
                "activity": activity,
                "begin": begin,
                "end": end,
                "description": description,
            },
        )
        response.raise_for_status()
        return response.json()


def sync_repo_times(
    *,
    forgejo: ForgejoClient,
    kimai: KimaiClient,
    owner: str,
    repo: str,
    kimai_project: int,
    kimai_activity: int,
) -> list[dict]:
    """Pushes every Forgejo time entry for owner/repo that isn't already in Kimai.

    Idempotency key is the Forgejo time-entry ID, embedded in the Kimai
    timesheet's description. Returns the timesheets created this run.
    """
    already_synced = {
        entry["description"].removeprefix(DESCRIPTION_PREFIX).split()[0]
        for entry in kimai.list_timesheets()
        if entry.get("description", "").startswith(DESCRIPTION_PREFIX)
    }

    created = []
    for entry in forgejo.list_repo_times(owner, repo):
        entry_id = str(entry["id"])
        if entry_id in already_synced:
            continue

        begin, end = _kimai_interval(entry["created"], entry["time"])
        issue_number = entry["issue"]["number"]
        description = f"{DESCRIPTION_PREFIX}{entry_id} issue:{owner}/{repo}#{issue_number}"

        created.append(
            kimai.create_timesheet(
                project=kimai_project,
                activity=kimai_activity,
                begin=begin,
                end=end,
                description=description,
            )
        )

    return created


def _kimai_interval(created_iso: str, duration_seconds: int) -> tuple[str, str]:
    """Aligns begin/end to the minute ourselves, rather than let Kimai do it.

    Kimai stores timesheets at minute granularity: it floors whatever begin
    it's given and ceils whatever end it's given. Left alone, that rounds the
    interval *outward* on both ends - a 90-minute entry that doesn't start on
    an exact minute boundary (the normal case, since Forgejo's timestamp is
    just "when the API call landed") comes back up to a minute longer. Doing
    the same floor/round ourselves first makes Kimai's rounding a no-op, so
    the duration that lands in Kimai matches what we intended to send.
    """
    from datetime import datetime, timedelta

    begin = datetime.fromisoformat(created_iso.replace("Z", "+00:00")).replace(
        second=0, microsecond=0
    )
    rounded_duration = round(duration_seconds / 60) * 60
    end = begin + timedelta(seconds=rounded_duration)
    return begin.isoformat(), end.isoformat()
