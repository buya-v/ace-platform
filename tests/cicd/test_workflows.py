"""
Tests for GitHub Actions workflow files.

Validates YAML structure, required fields, security practices,
and consistency across all workflow definitions.
"""

import os
import glob
import pytest
import yaml

WORKFLOW_DIR = os.path.join(
    os.path.dirname(__file__), "..", "..", ".github", "workflows"
)


def get_workflow_files():
    """Return all workflow YAML files."""
    pattern = os.path.join(WORKFLOW_DIR, "*.yml")
    files = glob.glob(pattern)
    assert len(files) > 0, f"No workflow files found in {WORKFLOW_DIR}"
    return files


def load_workflow(path):
    """Load and parse a workflow YAML file."""
    with open(path) as f:
        return yaml.safe_load(f)


@pytest.fixture(params=get_workflow_files(), ids=lambda p: os.path.basename(p))
def workflow(request):
    """Parameterized fixture yielding (filename, parsed_workflow) tuples."""
    path = request.param
    return os.path.basename(path), load_workflow(path)


class TestWorkflowStructure:
    """Validate basic workflow structure."""

    def test_has_name(self, workflow):
        filename, wf = workflow
        assert "name" in wf, f"{filename} missing 'name' field"

    def test_has_on_trigger(self, workflow):
        filename, wf = workflow
        assert "on" in wf or True in wf, f"{filename} missing 'on' trigger"

    def test_has_jobs(self, workflow):
        filename, wf = workflow
        assert "jobs" in wf, f"{filename} missing 'jobs' field"
        assert len(wf["jobs"]) > 0, f"{filename} has empty 'jobs'"

    def test_all_jobs_have_runs_on(self, workflow):
        filename, wf = workflow
        for job_name, job in wf["jobs"].items():
            assert "runs-on" in job, (
                f"{filename}: job '{job_name}' missing 'runs-on'"
            )

    def test_all_jobs_have_steps(self, workflow):
        filename, wf = workflow
        for job_name, job in wf["jobs"].items():
            assert "steps" in job, (
                f"{filename}: job '{job_name}' missing 'steps'"
            )
            assert len(job["steps"]) > 0, (
                f"{filename}: job '{job_name}' has empty 'steps'"
            )


class TestSecurityPractices:
    """Validate security best practices in workflows."""

    def test_checkout_uses_pinned_action(self, workflow):
        """All checkout actions should use a versioned tag."""
        filename, wf = workflow
        for job_name, job in wf["jobs"].items():
            for step in job["steps"]:
                if "uses" in step and "actions/checkout" in step["uses"]:
                    assert "@v" in step["uses"], (
                        f"{filename}: job '{job_name}' uses unpinned checkout"
                    )

    def test_no_plaintext_secrets(self, workflow):
        """No hardcoded secrets or tokens in workflow files."""
        filename, wf = workflow
        # Read raw file to check for actual hardcoded values
        path = os.path.join(WORKFLOW_DIR, filename)
        with open(path) as f:
            raw = f.read()
        # Check for AWS access key patterns
        assert "AKIA" not in raw, (
            f"{filename} may contain hardcoded AWS access key"
        )
        # Check for inline password/key assignments (not ${{ secrets.* }} refs)
        import re
        # Match "key: value" where value is NOT a secrets/vars reference
        for pattern_name, pattern in [
            ("password", r'password:\s*["\']?[A-Za-z0-9]'),
            ("api_key", r'api_key:\s*["\']?[A-Za-z0-9]'),
        ]:
            matches = re.findall(pattern, raw)
            assert len(matches) == 0, (
                f"{filename} may contain hardcoded {pattern_name}"
            )

    def test_aws_uses_oidc_not_keys(self, workflow):
        """AWS credentials should use OIDC role assumption, not access keys."""
        filename, wf = workflow
        raw = yaml.dump(wf)
        if "configure-aws-credentials" in raw:
            assert "role-to-assume" in raw, (
                f"{filename}: AWS auth should use role-to-assume (OIDC), "
                "not access keys"
            )
            assert "aws-access-key-id" not in raw, (
                f"{filename}: should not use static AWS access keys"
            )


class TestCIWorkflow:
    """Tests specific to the CI workflow."""

    def test_ci_exists(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "ci.yml"))
        assert wf is not None

    def test_ci_has_lint_test_build_jobs(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "ci.yml"))
        jobs = wf["jobs"]
        assert "lint" in jobs, "CI missing lint job"
        assert "test" in jobs, "CI missing test job"
        assert "build" in jobs, "CI missing build job"

    def test_ci_has_gate_job(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "ci.yml"))
        assert "ci-gate" in wf["jobs"], "CI missing gate job"

    def test_ci_triggers_on_pr(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "ci.yml"))
        triggers = wf.get("on") or wf.get(True, {})
        assert "pull_request" in triggers, "CI should trigger on pull_request"

    def test_ci_has_concurrency(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "ci.yml"))
        assert "concurrency" in wf, "CI should use concurrency to cancel stale runs"


class TestSecurityWorkflow:
    """Tests specific to the security workflow."""

    def test_security_exists(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "security.yml"))
        assert wf is not None

    def test_security_has_codeql(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "security.yml"))
        assert "codeql" in wf["jobs"], "Security missing CodeQL job"

    def test_security_has_trivy(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "security.yml"))
        job_names = " ".join(wf["jobs"].keys())
        assert "trivy" in job_names, "Security missing Trivy scan job"

    def test_security_has_secret_detection(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "security.yml"))
        assert "secret-scan" in wf["jobs"], "Security missing secret detection"

    def test_security_has_scheduled_run(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "security.yml"))
        triggers = wf.get("on") or wf.get(True, {})
        assert "schedule" in triggers, "Security should have scheduled runs"


class TestTerraformWorkflow:
    """Tests specific to the Terraform workflow."""

    def test_terraform_exists(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        assert wf is not None

    def test_terraform_has_validate_job(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        assert "terraform-validate" in wf["jobs"]

    def test_terraform_has_plan_job(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        assert "terraform-plan" in wf["jobs"]

    def test_terraform_plans_all_environments(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        plan = wf["jobs"]["terraform-plan"]
        matrix = plan.get("strategy", {}).get("matrix", {})
        envs = matrix.get("environment", [])
        assert "dev" in envs
        assert "staging" in envs
        assert "prod" in envs

    def test_terraform_apply_sequential(self):
        """Apply jobs should run sequentially: dev -> staging -> prod."""
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        jobs = wf["jobs"]

        # staging depends on dev
        staging_needs = jobs.get("terraform-apply-staging", {}).get("needs", [])
        if isinstance(staging_needs, str):
            staging_needs = [staging_needs]
        assert "terraform-apply-dev" in staging_needs

        # prod depends on staging
        prod_needs = jobs.get("terraform-apply-prod", {}).get("needs", [])
        if isinstance(prod_needs, str):
            prod_needs = [prod_needs]
        assert "terraform-apply-staging" in prod_needs

    def test_terraform_apply_uses_environments(self):
        """Apply jobs should use GitHub environments for approval gates."""
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "terraform.yml"))
        for job_key in ["terraform-apply-dev", "terraform-apply-staging", "terraform-apply-prod"]:
            job = wf["jobs"].get(job_key, {})
            assert "environment" in job, (
                f"{job_key} should use a GitHub environment for approval gates"
            )


class TestDockerBuildWorkflow:
    """Tests specific to the Docker build workflow."""

    def test_docker_exists(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "docker-build.yml"))
        assert wf is not None

    def test_docker_has_build_push_job(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "docker-build.yml"))
        assert "build-push" in wf["jobs"]

    def test_docker_uses_buildx(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "docker-build.yml"))
        job = wf["jobs"]["build-push"]
        step_uses = [s.get("uses", "") for s in job["steps"]]
        assert any("buildx" in u for u in step_uses), "Should use Docker Buildx"

    def test_docker_scans_built_images(self):
        """Built images should be scanned for vulnerabilities."""
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "docker-build.yml"))
        job = wf["jobs"]["build-push"]
        step_uses = [s.get("uses", "") for s in job["steps"]]
        assert any("trivy" in u for u in step_uses), (
            "Built images should be scanned with Trivy"
        )


class TestDeployWorkflow:
    """Tests specific to the deploy workflow."""

    def test_deploy_exists(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "deploy.yml"))
        assert wf is not None

    def test_deploy_has_environment_gate(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "deploy.yml"))
        deploy_job = wf["jobs"].get("deploy", {})
        assert "environment" in deploy_job, (
            "Deploy job should use GitHub environment for approval"
        )

    def test_deploy_supports_manual_trigger(self):
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "deploy.yml"))
        triggers = wf.get("on") or wf.get(True, {})
        assert "workflow_dispatch" in triggers, (
            "Deploy should support manual trigger"
        )

    def test_deploy_promotion_chain(self):
        """Deploy should promote: dev -> staging -> prod."""
        wf = load_workflow(os.path.join(WORKFLOW_DIR, "deploy.yml"))
        jobs = wf["jobs"]
        assert "promote-staging" in jobs, "Missing staging promotion"
        assert "promote-prod" in jobs, "Missing prod promotion"


class TestBranchProtection:
    """Tests for branch protection configuration."""

    def test_branch_protection_file_exists(self):
        path = os.path.join(
            os.path.dirname(__file__), "..", "..", ".github", "branch-protection.json"
        )
        assert os.path.exists(path), "Branch protection config not found"

    def test_branch_protection_valid_json(self):
        import json
        path = os.path.join(
            os.path.dirname(__file__), "..", "..", ".github", "branch-protection.json"
        )
        with open(path) as f:
            config = json.load(f)
        assert "rules" in config
        assert "main" in config["rules"]

    def test_main_requires_pr_reviews(self):
        import json
        path = os.path.join(
            os.path.dirname(__file__), "..", "..", ".github", "branch-protection.json"
        )
        with open(path) as f:
            config = json.load(f)
        main = config["rules"]["main"]
        reviews = main["required_pull_request_reviews"]
        assert reviews["required_approving_review_count"] >= 1

    def test_main_requires_status_checks(self):
        import json
        path = os.path.join(
            os.path.dirname(__file__), "..", "..", ".github", "branch-protection.json"
        )
        with open(path) as f:
            config = json.load(f)
        main = config["rules"]["main"]
        checks = main["required_status_checks"]["contexts"]
        assert "CI Gate" in checks
        assert "Security Gate" in checks

    def test_main_disallows_force_push(self):
        import json
        path = os.path.join(
            os.path.dirname(__file__), "..", "..", ".github", "branch-protection.json"
        )
        with open(path) as f:
            config = json.load(f)
        assert config["rules"]["main"]["allow_force_pushes"] is False
