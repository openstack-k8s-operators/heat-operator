"""Check Heat's Zuul image selection without building or deploying containers."""

import itertools
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml
from ansible.parsing.splitter import split_args

ROOT = Path(__file__).resolve().parents[2]
S2I_PROJECT = "github.com/openstack-k8s-operators/s2i-openstack-containers"
HEAT_PROJECT = "github.com/openstack-k8s-operators/heat-operator"
JOBS = {
    entry["job"]["name"]: entry["job"]
    for entry in yaml.safe_load((ROOT / "zuul.d/jobs.yaml").read_text())
}
CONSUMER = JOBS["heat-operator-tempest-multinode"]
PROVIDER = JOBS["heat-s2i-content-provider"]
PUBLISHED = {
    "heatAPIImage": "quay.io/openstack-s2i-containers/openstack-heat-api:master-latest",
    "heatCfnapiImage": "quay.io/openstack-s2i-containers/openstack-heat-api:master-latest",
    "heatEngineImage": "quay.io/openstack-s2i-containers/openstack-heat-engine:master-latest",
}


def zuul_vars(*projects):
    return {
        "items": [{"project": {"canonical_name": project}} for project in projects],
        # A required checkout alone must not trigger a speculative build.
        "projects": {S2I_PROJECT: {"src_dir": "src/" + S2I_PROJECT}},
    }


class HeatImageSelectionTest(unittest.TestCase):
    def run_ansible(self, *args):
        result = subprocess.run(
            ["ansible-playbook", *map(str, args)],
            capture_output=True,
            text=True,
            env={**os.environ, "ANSIBLE_NOCOLOR": "1"},
            check=False,
        )
        return result

    def check_images(self, projects, expected, provider_vars=None, fail=False):
        with tempfile.TemporaryDirectory(prefix="heat-ci-") as directory:
            playbook = Path(directory) / "check.yaml"
            variables = {
                **CONSUMER["vars"],
                "zuul": zuul_vars(*projects),
                "expected_build": S2I_PROJECT in projects,
                "expected_images": expected,
                "expected_registries": (
                    ["192.0.2.1:5001"] if S2I_PROJECT in projects else []
                ),
                **(provider_vars or {}),
            }
            assertions = [
                "heat_s2i_build_required | bool == expected_build",
                "cifmw_crc_additional_insecure_registries == expected_registries",
                "cifmw_crc_additional_allowed_registries == expected_registries",
                "cifmw_set_containers_images | length == 3",
                "cifmw_set_containers_images | items2dict(key_name='name', "
                "value_name='full_registry') == expected_images",
            ]
            play = {
                "name": "Check actual job expressions",
                "hosts": "localhost",
                "connection": "local",
                "gather_facts": False,
                "vars": variables,
                "tasks": [{"ansible.builtin.assert": {"that": assertions}}],
            }
            playbook.write_text(yaml.safe_dump([play]))
            result = self.run_ansible("-i", "localhost,", playbook)
            if fail:
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn("undefined", (result.stdout + result.stderr).lower())
            else:
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_published_images_without_provider_metadata(self):
        for projects in [
            (),
            (HEAT_PROJECT,),
            (HEAT_PROJECT, "github.com/example/other"),
        ]:
            with self.subTest(projects=projects):
                self.check_images(projects, PUBLISHED)

    def test_speculative_images(self):
        images = {
            field: "192.0.2.1:5001/openstack/"
            + image.rsplit("/", 1)[1].replace("master-latest", "dependent-change")
            for field, image in PUBLISHED.items()
        }
        self.check_images(
            (HEAT_PROJECT, S2I_PROJECT),
            images,
            {
                "s2i_content_provider_registry_ip": "192.0.2.1",
                "s2i_content_provider_os_custom_container_images": images,
            },
        )

    def test_missing_speculative_metadata_does_not_fall_back(self):
        for provider_vars in [
            {},
            {"s2i_content_provider_registry_ip": "192.0.2.1"},
        ]:
            with self.subTest(provider_vars=provider_vars):
                self.check_images(
                    (HEAT_PROJECT, S2I_PROJECT),
                    PUBLISHED,
                    provider_vars,
                    fail=True,
                )

    def test_provider_import_is_one_legacy_parser_argument(self):
        # Ansible <= 2.18 splits import_playbook into a filename and parameters.
        # Leading Jinja set blocks become the filename and render to an empty
        # string, even though the complete template works on newer Ansible.
        wrapper = yaml.safe_load((ROOT / PROVIDER["run"]).read_text())
        imported = wrapper[1]["ansible.builtin.import_playbook"]
        self.assertEqual(split_args(imported), [imported])

    def test_provider_builds_only_for_s2i_change(self):
        for build_required, tagged_inventory in itertools.product(
            (False, True), repeat=2
        ):
            with self.subTest(
                build_required=build_required,
                tagged_inventory=tagged_inventory,
            ), tempfile.TemporaryDirectory(prefix="heat-ci-") as directory:
                work_root = Path(directory)
                upstream = (
                    work_root
                    / "src"
                    / S2I_PROJECT
                    / "playbooks/container-ci/zuul/run.yaml"
                )
                upstream.parent.mkdir(parents=True)
                marker = work_root / "built"
                # Exercise the real wrapper/import/when, replacing only the
                # external build playbook to avoid actual container builds.
                build_task = {
                    "ansible.builtin.copy": {
                        "content": "built",
                        "dest": str(marker),
                        "mode": "0600",
                    }
                }
                play = {
                    "hosts": "builder",
                    "gather_facts": False,
                    "tasks": [build_task],
                }
                upstream.write_text(yaml.safe_dump([play]))
                zuul = zuul_vars(
                    HEAT_PROJECT, *([S2I_PROJECT] if build_required else [])
                )
                zuul["executor"] = {"work_root": str(work_root)}
                zuul["change_message"] = "change-message-placeholder"
                inventory = work_root / "inventory.yaml"
                group = {
                    "hosts": {"builder": {"ansible_connection": "local"}},
                    "vars": {
                        **PROVIDER["vars"],
                        "unsafe_vars": {"example": "job-var-placeholder"},
                    },
                }
                # Zuul passes executor metadata through -e @zuul_vars.yaml,
                # not through the per-playbook execution inventory. This also
                # makes it available when static imports are parsed.
                extra_vars = work_root / "zuul_vars.yaml"
                extra_vars_text = yaml.safe_dump({"zuul": zuul})
                inventory_text = yaml.safe_dump({"all": group})
                if tagged_inventory:
                    # Zuul protects PR messages and original job variables
                    # with !unsafe. They must never be evaluated as templates.
                    extra_vars_text = extra_vars_text.replace(
                        "change-message-placeholder",
                        "!unsafe '{{ unsafe_message_must_not_be_evaluated }}'",
                    )
                    inventory_text = inventory_text.replace(
                        "job-var-placeholder",
                        "!unsafe '{{ unsafe_job_var_must_not_be_evaluated }}'",
                    )
                extra_vars.write_text(extra_vars_text)
                inventory.write_text(inventory_text)
                result = self.run_ansible(
                    "-i", inventory, "-e", "@" + str(extra_vars), ROOT / PROVIDER["run"]
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(marker.exists(), build_required)
                self.assertEqual(inventory.read_text(), inventory_text)
                self.assertEqual(extra_vars.read_text(), extra_vars_text)

    def test_single_voting_consumer_and_provider_dependencies(self):
        project = yaml.safe_load((ROOT / "zuul.d/project.yaml").read_text())[0][
            "project"
        ]
        scheduled = project["github-check"]["jobs"]
        self.assertIn("heat-operator-tempest-multinode", scheduled)
        self.assertIn("heat-s2i-content-provider", scheduled)
        self.assertNotIn("heat-s2i-test", scheduled)
        self.assertNotIn("heat-s2i-test", JOBS)
        self.assertEqual(CONSUMER["parent"], "s2i-test-base")
        self.assertEqual(
            CONSUMER["nodeset"],
            "centos-10-medium-2x-centos-10-crc-cloud-ocp-4-22-1-3xl",
        )
        self.assertEqual(
            CONSUMER["dependencies"],
            [
                "openstack-k8s-operators-content-provider",
                "heat-s2i-content-provider",
            ],
        )
        self.assertEqual(len(CONSUMER["vars"]["s2i_extras"]), 1)


if __name__ == "__main__":
    unittest.main()
