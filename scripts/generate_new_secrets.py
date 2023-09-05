#/usr/bin/env python
import os
import sys
import shutil
import binascii
from subprocess import call
from typing import Iterator
from pathlib import Path

from kubernetes import client, config
from base64 import b64decode, b64encode

from yaml import load, load_all, SafeLoader, dump, SafeDumper
from yaml.composer import ComposerError

config.load_kube_config()
V1 = client.CoreV1Api()
CLIENT = client.ApiClient()

SCRIPTPATH = Path(__file__).parent
PATH = SCRIPTPATH.parent

ENVIRONMENTS = ("development", "staging", "production")
try:
    ENVIRONMENT = sys.argv[1]
    assert ENVIRONMENT.lower() in ENVIRONMENTS, f"Environment must be one of {ENVIRONMENTS}"
except IndexError:
    raise AssertionError(f"Environment must be one of {ENVIRONMENTS}")


def namespace():
    return f"etos-{ENVIRONMENT}"


def directories(environment: str, path: Path) -> Iterator[Path] :
    for (dirpath, dirnames, filenames) in os.walk(path):
        if dirpath.endswith(f"overlays/{environment}"):
            yield Path(dirpath)


def files(path: Path):
    for (dirpath, dirnames, filenames) in os.walk(path):
        for filename in filenames:
            if filename.endswith(".yaml"):
                yield Path(dirpath).joinpath(filename)


def secret_names(yaml_data: list[dict]):
    for filepath, yaml in yaml_data:
        if yaml["kind"] == "SealedSecret":
            yield filepath, yaml["spec"]["template"]["metadata"]["name"]


def load_yaml(environment, path):
    yamldata = []
    for directory in directories(environment, path):
        for _file in files(directory):
            with _file.open() as open_file:
                try:
                    for yaml in load_all(open_file, Loader=SafeLoader):
                        yamldata.append((_file, yaml))
                except (ComposerError, UnicodeDecodeError):
                    print(f"Failed to load: {FILE}")
    return yamldata


def active_secrets(namespace):
    response = V1.list_namespaced_secret(namespace)
    active_secrets = {}
    for secret in response.items:
        active_secrets[secret.metadata.name] = CLIENT.sanitize_for_serialization(secret)
    return active_secrets


def clear(path):
    shutil.rmtree(path, ignore_errors=True)


def path(name):
    secretpath = SCRIPTPATH.joinpath(name)
    clear(secretpath)
    os.makedirs(secretpath)
    return secretpath


def decode(secret):
    for key, value in secret["data"].items():
        secret["data"][key] = b64decode(value).decode()
    return secret


def encode(secret):
    for key, value in secret["data"].items():
        secret["data"][key] = b64encode(value.encode()).decode()
    return secret


def secrets(yaml_data, secretpath, active, namespace):
    secrets = []
    for filepath, sealed_secret in secret_names(yaml_data):
        secret = active.get(sealed_secret)
        secret["metadata"].pop("managedFields")
        secret["metadata"].pop("ownerReferences")
        secret["metadata"].pop("resourceVersion")
        secret["metadata"].pop("uid")
        name = secretpath.joinpath(f"{secret['metadata']['name']}_{namespace}.yaml")
        with open(name, mode="w") as yamlfile:
            dump(decode(secret), yamlfile)
        secrets.append((filepath, name))
    return secrets


def fix_metadata(old_sealed_secret_path, new_sealed_secret_path):
    with open(old_sealed_secret_path) as sealed_secret_yaml:
        old_sealed_secret = load(sealed_secret_yaml, Loader=SafeLoader)
    with open(new_sealed_secret_path) as sealed_secret_yaml:
        new_sealed_secret = load(sealed_secret_yaml, Loader=SafeLoader)
    if old_sealed_secret["metadata"].get("labels"):
        new_sealed_secret["metadata"]["labels"] = old_sealed_secret["metadata"]["labels"]
    try:
        new_sealed_secret["metadata"].pop("namespace")
    except KeyError:
        pass
    try:
        new_sealed_secret["spec"]["template"]["metadata"].pop("namespace")
    except KeyError:
        pass
    return new_sealed_secret


def encode_secret(_file):
    with _file.open() as secret_file:
        data = load(secret_file, Loader=SafeLoader)
    with _file.open(mode="w") as secret_file:
        dump(encode(data), secret_file)


def generate_sealed_secrets(updated_secrets, sealed_secret_dir):
    sealed_secrets = []
    for sealed_secret_path, _file in updated_secrets:
        if not _file.exists():
            continue
        new_secret = sealed_secret_dir.joinpath(_file.name)
        encode_secret(_file)
        cmd = f"kubeseal --controller-namespace sealed-secrets --controller-name sealed-secrets --scope cluster-wide --format yaml < {_file} > {sealed_secret_dir}/{_file.name}"
        call(cmd, shell=True)
        new_sealed_secret = fix_metadata(sealed_secret_path, new_secret)
        with open(new_secret, "w") as sealed_secret_yaml:
            dump(new_sealed_secret, sealed_secret_yaml)
        sealed_secrets.append((new_secret, sealed_secret_path))
    return sealed_secrets


def copy(secret_pairs):
    for new_sealed_secret, old_sealed_secret in secret_pairs:
        shutil.copyfile(new_sealed_secret, old_sealed_secret)


if __name__ == "__main__":
    NAMESPACE = namespace()
    SECRETPATH = path("secrets")
    SEALEDSECRETPATH = path("sealed_secrets")
    SECRETS = secrets(load_yaml(ENVIRONMENT, PATH), SECRETPATH, active_secrets(NAMESPACE), NAMESPACE)

    print(f"Files are now located here: ./{SECRETPATH.relative_to(Path.cwd())}")
    print("Please make edits to all that need, and delete the ones that you don't want to update.")
    print("Write the messages in plaintext, we will encode them!")
    input("Press 'enter' to continue, or Ctrl+C to abort.")

    NEW_SECRETS = generate_sealed_secrets(SECRETS, SEALEDSECRETPATH)

    print()
    print(f"Sealed secrets have been generated here: ./{SEALEDSECRETPATH.relative_to(Path.cwd())}")
    print("Ready to overwrite old sealed secret files in this repository.")
    input("Press 'enter' to continue, or Ctrl+C to abort.")

    copy(NEW_SECRETS)

    clear(SCRIPTPATH.joinpath("secrets"))
    clear(SCRIPTPATH.joinpath("sealed_secrets"))
    print()
    print(f"Sealed secrets for {ENVIRONMENT!r} have been updated")
