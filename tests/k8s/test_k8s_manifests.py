"""Tests for Kubernetes deployment manifests.

Validates YAML structure, required fields, port assignments,
resource limits, and overlay consistency for all ACE platform services.
"""

import os
import unittest

import yaml

BASE_DIR = os.path.join(
    os.path.dirname(__file__), "..", "..", "deploy", "k8s", "base"
)
OVERLAYS_DIR = os.path.join(
    os.path.dirname(__file__), "..", "..", "deploy", "k8s", "overlays"
)

# Service definitions: name -> (namespace, grpc_port, health_port)
GRPC_SERVICES = {
    "matching-engine": ("ace-exchange", 50051, 8081),
    "clearing-engine": ("ace-exchange", 50052, 8082),
    "margin-engine": ("ace-exchange", 50053, 8083),
    "settlement-engine": ("ace-exchange", 50054, 8084),
    "auth-service": ("ace-services", 50055, 8085),
    "compliance-service": ("ace-services", 50056, 8086),
    "market-data-service": ("ace-services", 50057, 8087),
    "warehouse-service": ("ace-services", 50058, 8088),
}

GATEWAY_PORTS = {"http": 8080, "health": 8090}

UI_SERVICES = ["web-ui", "admin-ui"]

INFRA_SERVICES = ["postgres", "redis", "kafka", "zookeeper"]


def load_yaml(filepath):
    """Load all YAML documents from a file."""
    with open(filepath) as f:
        return list(yaml.safe_load_all(f))


def load_single_yaml(filepath):
    """Load a single YAML document from a file."""
    with open(filepath) as f:
        return yaml.safe_load(f)


class TestDirectoryStructure(unittest.TestCase):
    """Test that all expected directories and files exist."""

    def test_base_directory_exists(self):
        self.assertTrue(os.path.isdir(BASE_DIR))

    def test_overlay_directories_exist(self):
        for env in ["dev", "staging", "prod"]:
            path = os.path.join(OVERLAYS_DIR, env)
            self.assertTrue(os.path.isdir(path), f"Missing overlay: {env}")

    def test_grpc_service_directories(self):
        for svc in GRPC_SERVICES:
            path = os.path.join(BASE_DIR, svc)
            self.assertTrue(os.path.isdir(path), f"Missing service dir: {svc}")

    def test_infra_directories(self):
        for svc in INFRA_SERVICES:
            path = os.path.join(BASE_DIR, svc)
            self.assertTrue(os.path.isdir(path), f"Missing infra dir: {svc}")

    def test_ui_directories(self):
        for svc in UI_SERVICES:
            path = os.path.join(BASE_DIR, svc)
            self.assertTrue(os.path.isdir(path), f"Missing UI dir: {svc}")

    def test_grpc_service_files(self):
        expected_files = [
            "deployment.yaml",
            "service.yaml",
            "serviceaccount.yaml",
            "hpa.yaml",
            "pdb.yaml",
            "kustomization.yaml",
        ]
        for svc in GRPC_SERVICES:
            for f in expected_files:
                path = os.path.join(BASE_DIR, svc, f)
                self.assertTrue(
                    os.path.isfile(path), f"Missing {f} in {svc}"
                )

    def test_gateway_files(self):
        expected = [
            "deployment.yaml",
            "service.yaml",
            "serviceaccount.yaml",
            "hpa.yaml",
            "pdb.yaml",
            "kustomization.yaml",
        ]
        for f in expected:
            path = os.path.join(BASE_DIR, "gateway", f)
            self.assertTrue(os.path.isfile(path), f"Missing gateway/{f}")


class TestGRPCServiceManifests(unittest.TestCase):
    """Validate gRPC service deployment manifests."""

    def test_deployment_ports(self):
        for svc, (ns, grpc_port, health_port) in GRPC_SERVICES.items():
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            dep = docs[0]
            self.assertEqual(dep["kind"], "Deployment")
            self.assertEqual(dep["metadata"]["namespace"], ns)

            containers = dep["spec"]["template"]["spec"]["containers"]
            self.assertEqual(len(containers), 1)
            ports = {p["name"]: p["containerPort"] for p in containers[0]["ports"]}
            self.assertEqual(ports["grpc"], grpc_port, f"{svc} grpc port mismatch")
            self.assertEqual(ports["health"], health_port, f"{svc} health port mismatch")

    def test_deployment_probes(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            container = docs[0]["spec"]["template"]["spec"]["containers"][0]

            self.assertIn("readinessProbe", container)
            self.assertIn("livenessProbe", container)
            self.assertEqual(
                container["readinessProbe"]["httpGet"]["path"], "/healthz"
            )
            self.assertEqual(
                container["livenessProbe"]["httpGet"]["path"], "/healthz"
            )

    def test_deployment_resources(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            resources = docs[0]["spec"]["template"]["spec"]["containers"][0]["resources"]
            self.assertIn("requests", resources)
            self.assertIn("limits", resources)
            self.assertIn("memory", resources["requests"])
            self.assertIn("cpu", resources["requests"])

    def test_deployment_service_account(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            sa = docs[0]["spec"]["template"]["spec"]["serviceAccountName"]
            self.assertEqual(sa, svc)

    def test_deployment_env_from(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            env_from = docs[0]["spec"]["template"]["spec"]["containers"][0]["envFrom"]
            config_refs = [
                e["configMapRef"]["name"]
                for e in env_from
                if "configMapRef" in e
            ]
            self.assertIn("service-discovery", config_refs)

    def test_service_ports(self):
        for svc, (ns, grpc_port, health_port) in GRPC_SERVICES.items():
            docs = load_yaml(os.path.join(BASE_DIR, svc, "service.yaml"))
            svc_obj = docs[0]
            self.assertEqual(svc_obj["kind"], "Service")
            self.assertEqual(svc_obj["spec"]["type"], "ClusterIP")
            ports = {p["name"]: p["port"] for p in svc_obj["spec"]["ports"]}
            self.assertEqual(ports["grpc"], grpc_port)
            self.assertEqual(ports["health"], health_port)

    def test_service_account_irsa(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "serviceaccount.yaml"))
            sa = docs[0]
            self.assertEqual(sa["kind"], "ServiceAccount")
            annotations = sa["metadata"].get("annotations", {})
            self.assertIn("eks.amazonaws.com/role-arn", annotations)

    def test_hpa_spec(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "hpa.yaml"))
            hpa = docs[0]
            self.assertEqual(hpa["kind"], "HorizontalPodAutoscaler")
            self.assertEqual(hpa["spec"]["scaleTargetRef"]["name"], svc)
            self.assertGreater(hpa["spec"]["minReplicas"], 0)
            self.assertGreater(hpa["spec"]["maxReplicas"], hpa["spec"]["minReplicas"])

    def test_pdb_spec(self):
        for svc in GRPC_SERVICES:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "pdb.yaml"))
            pdb = docs[0]
            self.assertEqual(pdb["kind"], "PodDisruptionBudget")
            self.assertEqual(pdb["spec"]["maxUnavailable"], 1)

    def test_no_port_collisions(self):
        """Ensure no two services share the same gRPC or health port."""
        grpc_ports = {}
        health_ports = {}
        for svc, (_, grpc_port, health_port) in GRPC_SERVICES.items():
            self.assertNotIn(
                grpc_port,
                grpc_ports,
                f"gRPC port {grpc_port} collision: {svc} vs {grpc_ports.get(grpc_port)}",
            )
            self.assertNotIn(
                health_port,
                health_ports,
                f"Health port {health_port} collision: {svc} vs {health_ports.get(health_port)}",
            )
            grpc_ports[grpc_port] = svc
            health_ports[health_port] = svc


class TestGatewayManifests(unittest.TestCase):
    """Validate gateway deployment manifests."""

    def test_gateway_ports(self):
        docs = load_yaml(os.path.join(BASE_DIR, "gateway", "deployment.yaml"))
        container = docs[0]["spec"]["template"]["spec"]["containers"][0]
        ports = {p["name"]: p["containerPort"] for p in container["ports"]}
        self.assertEqual(ports["http"], 8080)
        self.assertEqual(ports["health"], 8090)

    def test_gateway_service_type(self):
        docs = load_yaml(os.path.join(BASE_DIR, "gateway", "service.yaml"))
        self.assertEqual(docs[0]["spec"]["type"], "ClusterIP")


class TestInfraManifests(unittest.TestCase):
    """Validate infrastructure service manifests."""

    def test_postgres_statefulset(self):
        docs = load_yaml(os.path.join(BASE_DIR, "postgres", "statefulset.yaml"))
        ss = docs[0]
        self.assertEqual(ss["kind"], "StatefulSet")
        self.assertEqual(ss["metadata"]["namespace"], "ace-infra")
        vcts = ss["spec"]["volumeClaimTemplates"]
        self.assertEqual(len(vcts), 1)
        self.assertEqual(
            vcts[0]["spec"]["resources"]["requests"]["storage"], "10Gi"
        )

    def test_postgres_probe(self):
        docs = load_yaml(os.path.join(BASE_DIR, "postgres", "statefulset.yaml"))
        container = docs[0]["spec"]["template"]["spec"]["containers"][0]
        self.assertIn("readinessProbe", container)
        self.assertIn("pg_isready", container["readinessProbe"]["exec"]["command"])

    def test_redis_deployment(self):
        docs = load_yaml(os.path.join(BASE_DIR, "redis", "deployment.yaml"))
        dep = docs[0]
        self.assertEqual(dep["kind"], "Deployment")
        container = dep["spec"]["template"]["spec"]["containers"][0]
        ports = {p["name"]: p["containerPort"] for p in container["ports"]}
        self.assertEqual(ports["redis"], 6379)

    def test_kafka_statefulset(self):
        docs = load_yaml(os.path.join(BASE_DIR, "kafka", "statefulset.yaml"))
        ss = docs[0]
        self.assertEqual(ss["kind"], "StatefulSet")
        self.assertEqual(ss["spec"]["serviceName"], "kafka-headless")
        container = ss["spec"]["template"]["spec"]["containers"][0]
        ports = {p["name"]: p["containerPort"] for p in container["ports"]}
        self.assertEqual(ports["broker"], 9092)

    def test_zookeeper_statefulset(self):
        docs = load_yaml(os.path.join(BASE_DIR, "zookeeper", "statefulset.yaml"))
        ss = docs[0]
        self.assertEqual(ss["kind"], "StatefulSet")
        container = ss["spec"]["template"]["spec"]["containers"][0]
        ports = {p["name"]: p["containerPort"] for p in container["ports"]}
        self.assertEqual(ports["client"], 2181)

    def test_kafka_headless_service(self):
        docs = load_yaml(os.path.join(BASE_DIR, "kafka", "service.yaml"))
        headless = [d for d in docs if d["metadata"]["name"] == "kafka-headless"]
        self.assertEqual(len(headless), 1)
        # YAML None parses as Python None; "None" string also valid for K8s
        cluster_ip = headless[0]["spec"]["clusterIP"]
        self.assertTrue(
            cluster_ip is None or cluster_ip == "None",
            f"Expected None/headless, got {cluster_ip}",
        )

    def test_zookeeper_headless_service(self):
        docs = load_yaml(os.path.join(BASE_DIR, "zookeeper", "service.yaml"))
        headless = [d for d in docs if d["metadata"]["name"] == "zookeeper-headless"]
        self.assertEqual(len(headless), 1)
        cluster_ip = headless[0]["spec"]["clusterIP"]
        self.assertTrue(
            cluster_ip is None or cluster_ip == "None",
            f"Expected None/headless, got {cluster_ip}",
        )


class TestServiceDiscovery(unittest.TestCase):
    """Validate the service discovery ConfigMap."""

    def test_configmap_has_all_services(self):
        docs = load_yaml(os.path.join(BASE_DIR, "service-discovery-configmap.yaml"))
        cm = docs[0]
        self.assertEqual(cm["kind"], "ConfigMap")
        data = cm["data"]

        expected_keys = [
            "MATCHING_ENGINE_ADDR",
            "CLEARING_ENGINE_ADDR",
            "MARGIN_ENGINE_ADDR",
            "SETTLEMENT_ENGINE_ADDR",
            "AUTH_SERVICE_ADDR",
            "COMPLIANCE_SERVICE_ADDR",
            "MARKET_DATA_SERVICE_ADDR",
            "WAREHOUSE_SERVICE_ADDR",
            "GATEWAY_ADDR",
            "POSTGRES_HOST",
            "REDIS_HOST",
            "KAFKA_BROKERS",
        ]
        for key in expected_keys:
            self.assertIn(key, data, f"Missing key: {key}")

    def test_configmap_addresses_use_cluster_dns(self):
        docs = load_yaml(os.path.join(BASE_DIR, "service-discovery-configmap.yaml"))
        data = docs[0]["data"]
        for key, value in data.items():
            if key.endswith("_ADDR"):
                self.assertIn(
                    ".svc.cluster.local",
                    value,
                    f"{key} should use full cluster DNS",
                )

    def test_configmap_ports_match_services(self):
        docs = load_yaml(os.path.join(BASE_DIR, "service-discovery-configmap.yaml"))
        data = docs[0]["data"]

        for svc, (_, grpc_port, _) in GRPC_SERVICES.items():
            key = svc.upper().replace("-", "_") + "_ADDR"
            self.assertIn(str(grpc_port), data[key], f"{key} port mismatch")


class TestSecrets(unittest.TestCase):
    """Validate secrets contain placeholder values."""

    def test_secrets_have_placeholders(self):
        docs = load_yaml(os.path.join(BASE_DIR, "secrets.yaml"))
        for doc in docs:
            self.assertEqual(doc["kind"], "Secret")
            for key, value in doc.get("stringData", {}).items():
                if "PASSWORD" in key or "KEY" in key:
                    self.assertIn(
                        "CHANGE_ME",
                        value,
                        f"Secret {key} should have placeholder value",
                    )


class TestNamespaces(unittest.TestCase):
    """Validate namespace definitions."""

    def test_no_duplicate_namespace_file(self):
        """namespace.yaml should not exist — namespaces.yaml is the single source."""
        path = os.path.join(BASE_DIR, "namespace.yaml")
        self.assertFalse(os.path.isfile(path), "Duplicate namespace.yaml should be removed")

    def test_required_namespaces(self):
        docs = load_yaml(os.path.join(BASE_DIR, "namespaces.yaml"))
        ns_names = [d["metadata"]["name"] for d in docs]
        for ns in ["ace-exchange", "ace-services", "ace-infra"]:
            self.assertIn(ns, ns_names, f"Missing namespace: {ns}")


class TestKustomizationFiles(unittest.TestCase):
    """Validate kustomization.yaml files."""

    def test_base_kustomization_references_all_services(self):
        kust = load_single_yaml(os.path.join(BASE_DIR, "kustomization.yaml"))
        resources = kust["resources"]
        for svc in list(GRPC_SERVICES.keys()) + ["gateway"] + UI_SERVICES:
            ref = f"{svc}/"
            self.assertIn(ref, resources, f"Base kustomization missing {svc}")

        for infra in INFRA_SERVICES:
            ref = f"{infra}/"
            self.assertIn(ref, resources, f"Base kustomization missing {infra}")

    def test_overlay_kustomizations_reference_base(self):
        for env in ["dev", "staging", "prod"]:
            kust = load_single_yaml(
                os.path.join(OVERLAYS_DIR, env, "kustomization.yaml")
            )
            self.assertIn("../../base", kust.get("resources", []))

    def test_overlay_kustomizations_have_patches(self):
        for env in ["dev", "staging", "prod"]:
            kust = load_single_yaml(
                os.path.join(OVERLAYS_DIR, env, "kustomization.yaml")
            )
            self.assertIn("patches", kust, f"{env} overlay missing patches")
            self.assertGreater(
                len(kust["patches"]), 0, f"{env} overlay has no patches"
            )


class TestLabels(unittest.TestCase):
    """Validate consistent labeling across manifests."""

    def test_all_deployments_have_part_of_label(self):
        for svc in list(GRPC_SERVICES.keys()) + ["gateway"]:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            labels = docs[0]["metadata"]["labels"]
            self.assertEqual(labels["app.kubernetes.io/part-of"], "ace-platform")

    def test_selector_matches_template_labels(self):
        for svc in list(GRPC_SERVICES.keys()) + ["gateway"]:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            dep = docs[0]
            selector = dep["spec"]["selector"]["matchLabels"]
            template_labels = dep["spec"]["template"]["metadata"]["labels"]
            for k, v in selector.items():
                self.assertEqual(
                    template_labels.get(k),
                    v,
                    f"{svc}: selector label {k}={v} not in template",
                )


class TestCrossResourceValidation(unittest.TestCase):
    """Validate cross-resource references: every configMapRef/secretRef has a
    matching resource in the same namespace."""

    @classmethod
    def setUpClass(cls):
        """Build maps of available ConfigMaps and Secrets by (name, namespace)."""
        cls.configmaps = set()
        cls.secrets = set()

        # Parse service-discovery-configmap.yaml
        for doc in load_yaml(os.path.join(BASE_DIR, "service-discovery-configmap.yaml")):
            if doc and doc.get("kind") == "ConfigMap":
                cls.configmaps.add(
                    (doc["metadata"]["name"], doc["metadata"]["namespace"])
                )

        # Parse secrets.yaml
        for doc in load_yaml(os.path.join(BASE_DIR, "secrets.yaml")):
            if doc and doc.get("kind") == "Secret":
                cls.secrets.add(
                    (doc["metadata"]["name"], doc["metadata"]["namespace"])
                )

    def test_configmap_refs_resolvable(self):
        """Every configMapRef in a deployment must have a ConfigMap in the same namespace."""
        for svc in list(GRPC_SERVICES.keys()) + ["gateway"]:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            dep = docs[0]
            ns = dep["metadata"]["namespace"]
            containers = dep["spec"]["template"]["spec"]["containers"]
            for container in containers:
                for entry in container.get("envFrom", []):
                    if "configMapRef" in entry:
                        cm_name = entry["configMapRef"]["name"]
                        self.assertIn(
                            (cm_name, ns),
                            self.configmaps,
                            f"{svc}: configMapRef '{cm_name}' not found in namespace '{ns}'",
                        )

    def test_secret_refs_resolvable(self):
        """Every non-optional secretRef in a deployment must have a Secret in the same namespace."""
        for svc in list(GRPC_SERVICES.keys()) + ["gateway"]:
            docs = load_yaml(os.path.join(BASE_DIR, svc, "deployment.yaml"))
            dep = docs[0]
            ns = dep["metadata"]["namespace"]
            containers = dep["spec"]["template"]["spec"]["containers"]
            for container in containers:
                for entry in container.get("envFrom", []):
                    if "secretRef" in entry:
                        secret_name = entry["secretRef"]["name"]
                        optional = entry["secretRef"].get("optional", False)
                        if not optional:
                            self.assertIn(
                                (secret_name, ns),
                                self.secrets,
                                f"{svc}: secretRef '{secret_name}' not found in namespace '{ns}'",
                            )

    def test_jwt_secret_in_ace_exchange(self):
        """ace-exchange namespace must have ace-jwt-signing-key for engine JWT validation."""
        self.assertIn(
            ("ace-jwt-signing-key", "ace-exchange"),
            self.secrets,
            "ace-jwt-signing-key missing from ace-exchange namespace",
        )

    def test_service_discovery_configmap_per_namespace(self):
        """service-discovery ConfigMap must exist in both ace-exchange and ace-services."""
        for ns in ["ace-exchange", "ace-services"]:
            self.assertIn(
                ("service-discovery", ns),
                self.configmaps,
                f"service-discovery ConfigMap missing from {ns}",
            )

    def test_no_duplicate_namespace_definitions(self):
        """No namespace should be defined more than once across namespace files."""
        ns_file = os.path.join(BASE_DIR, "namespaces.yaml")
        docs = load_yaml(ns_file)
        ns_names = [d["metadata"]["name"] for d in docs if d]
        duplicates = [n for n in ns_names if ns_names.count(n) > 1]
        self.assertEqual(
            len(duplicates), 0,
            f"Duplicate namespace definitions: {set(duplicates)}",
        )


if __name__ == "__main__":
    unittest.main()
