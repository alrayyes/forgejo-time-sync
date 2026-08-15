"""Boots a real Forgejo container and hands back an admin token.

Forgejo needs no setup wizard: INSTALL_LOCK plus a handful of FORGEJO__ env
vars is enough for an unattended first boot, and `forgejo admin user create`
+ `generate-access-token` (both must run as the `git` user - root is refused)
does the rest.
"""

from dataclasses import dataclass

from testcontainers.core.container import DockerContainer, ExecConfig
from testcontainers.core.waiting_utils import wait_for_logs

ADMIN_USERNAME = "testowner"  # "admin" is a reserved Forgejo username


@dataclass(frozen=True)
class ForgejoInstance:
    container: DockerContainer
    base_url: str
    token: str


def start_forgejo() -> ForgejoInstance:
    container = (
        DockerContainer("codeberg.org/forgejo/forgejo:12")
        .with_env("USER_UID", "1000")
        .with_env("USER_GID", "1000")
        .with_env("FORGEJO__database__DB_TYPE", "sqlite3")
        .with_env("FORGEJO__security__INSTALL_LOCK", "true")
        .with_env("FORGEJO__service__DISABLE_REGISTRATION", "true")
        .with_exposed_ports(3000)
    )
    container.start()
    wait_for_logs(container, "Starting new Web server")

    host = container.get_container_host_ip()
    port = container.get_exposed_port(3000)
    base_url = f"http://{host}:{port}"

    container.exec(
        ExecConfig(
            command=[
                "forgejo",
                "admin",
                "user",
                "create",
                "--username",
                ADMIN_USERNAME,
                "--password",
                "adminpass123",
                "--email",
                "admin@example.test",
                "--admin",
            ],
            user="git",
        )
    )
    exit_code, output = container.exec(
        ExecConfig(
            command=[
                "forgejo",
                "admin",
                "user",
                "generate-access-token",
                "--username",
                ADMIN_USERNAME,
                "--token-name",
                "sync-test",
                "--scopes",
                "all",
            ],
            user="git",
        )
    )
    if exit_code != 0:
        raise RuntimeError(f"forgejo generate-access-token failed: {output.decode()}")
    # "Access token was successfully created: <token>"
    token = output.decode().strip().rsplit(":", 1)[1].strip()

    return ForgejoInstance(container=container, base_url=base_url, token=token)
