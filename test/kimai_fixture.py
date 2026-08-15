"""Boots a real Kimai container (against a real MySQL, its only supported
backend - the image's entrypoint parses DATABASE_URL as a MySQL DSN and has
no SQLite path despite what the docs imply) and hands back a working API
token.

There's no console command for provisioning an API token, and the UI route
that creates one is gated behind Kimai's first-login setup wizard - which
the API itself is deliberately exempt from (see WizardSubscriber: "never
trigger wizard on API calls"), but the page that *creates* a token is a UI
route, not an API one. So this walks the same steps a first-time user would:
log in, clear both wizard steps, then submit the (deprecated but still
functional, as of 2.65.0) API-password form to set a token.
"""

import re
from dataclasses import dataclass

import httpx
from testcontainers.core.container import DockerContainer
from testcontainers.core.network import Network
from testcontainers.core.waiting_utils import wait_for_logs

ADMIN_EMAIL = "admin@example.test"
ADMIN_PASSWORD = "adminpass123"
API_TOKEN = "test-api-token-123"
# The image's entrypoint always names the first user "admin" (it's the first
# positional arg to `kimai:user:create`, not derived from ADMINMAIL).
ADMIN_USERNAME = "admin"


@dataclass(frozen=True)
class KimaiInstance:
    mysql_container: DockerContainer
    kimai_container: DockerContainer
    network: Network
    base_url: str
    user: str
    token: str


def start_kimai() -> KimaiInstance:
    network = Network().create()

    mysql_container = (
        DockerContainer("mysql:8.4")
        .with_env("MYSQL_ROOT_PASSWORD", "rootpass")
        .with_env("MYSQL_DATABASE", "kimai")
        .with_env("MYSQL_USER", "kimai")
        .with_env("MYSQL_PASSWORD", "kimaipass")
        .with_network(network)
        .with_network_aliases("mysql")
    )
    mysql_container.start()
    wait_for_logs(mysql_container, "ready for connections", timeout=60)

    kimai_container = (
        DockerContainer("kimai/kimai2:apache")
        .with_env("DATABASE_URL", "mysql://kimai:kimaipass@mysql:3306/kimai")
        .with_env("ADMINMAIL", ADMIN_EMAIL)
        .with_env("ADMINPASS", ADMIN_PASSWORD)
        # A bare "*" is fed straight into Symfony's trusted-host regex and is
        # not a valid one (a lone quantifier); ".*" is.
        .with_env("TRUSTED_HOSTS", ".*")
        .with_env("APP_ENV", "prod")
        .with_network(network)
        .with_exposed_ports(8001)
    )
    kimai_container.start()
    wait_for_logs(kimai_container, "Kimai is ready", timeout=90)

    host = kimai_container.get_container_host_ip()
    port = kimai_container.get_exposed_port(8001)
    base_url = f"http://{host}:{port}"

    _wait_until_serving(base_url)
    _clear_wizard_and_set_token(base_url)

    return KimaiInstance(
        mysql_container=mysql_container,
        kimai_container=kimai_container,
        network=network,
        base_url=base_url,
        user=ADMIN_EMAIL,
        token=API_TOKEN,
    )


def _wait_until_serving(base_url: str, timeout: float = 60) -> None:
    import time

    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            httpx.get(f"{base_url}/en/login", timeout=5)
            return
        except httpx.HTTPError as error:  # noqa: PERF203
            last_error = error
            time.sleep(1)
    raise TimeoutError(f"Kimai never started serving: {last_error}")


def _clear_wizard_and_set_token(base_url: str) -> None:
    with httpx.Client(base_url=base_url, follow_redirects=False) as client:
        login_page = client.get("/en/login")
        csrf = _extract(login_page.text, r'name="_csrf_token" value="([^"]+)"')
        client.post(
            "/en/login_check",
            data={"_username": ADMIN_EMAIL, "_password": ADMIN_PASSWORD, "_csrf_token": csrf},
        )

        client.get("/en/wizard/intro")  # marks the "intro" step as seen

        profile_page = client.get("/en/wizard/profile")
        csrf = _extract(profile_page.text, r'name="form\[_token\]" value="([^"]+)"')
        client.post(
            "/en/wizard/profile",
            data={
                "form[language]": "en",
                "form[locale]": "en",
                "form[timezone]": "UTC",
                "form[skin]": "auto",
                "form[reload]": "0",
                "form[_token]": csrf,
            },
        )

        token_page = client.get(f"/en/profile/{ADMIN_USERNAME}/api-token")
        csrf = _extract(token_page.text, r'name="user_api_password\[_token\]" value="([^"]+)"')
        client.post(
            f"/en/profile/{ADMIN_USERNAME}/api-token",
            data={
                "user_api_password[plainApiToken][first]": API_TOKEN,
                "user_api_password[plainApiToken][second]": API_TOKEN,
                "user_api_password[_token]": csrf,
            },
        )


def _extract(html: str, pattern: str) -> str:
    match = re.search(pattern, html)
    if not match:
        raise RuntimeError(f"pattern not found in response: {pattern}")
    return match.group(1)
