# ── GarudaX Platform — Developer Commands ────────────────────────────────

.DEFAULT_GOAL := help

# ── Docker Infrastructure ────────────────────────────────────────────

.PHONY: up down logs ps clean

up:  ## Start infrastructure services (PostgreSQL, Redis, Kafka, MinIO)
	docker compose up -d

down:  ## Stop all services
	docker compose down

logs:  ## Follow logs for all services (use ARGS="garudax-postgres" for one service)
	docker compose logs -f $(ARGS)

ps:  ## Show running containers
	docker compose ps

clean:  ## Stop services and destroy all volumes (data loss!)
	docker compose down -v

# ── Database ─────────────────────────────────────────────────────────

.PHONY: db-shell db-schemas

db-shell:  ## Open psql shell to garudax-postgres
	docker exec -it garudax-postgres psql -U garudax_admin -d garudax_platform

db-schemas:  ## List database schemas
	docker exec -it garudax-postgres psql -U garudax_admin -d garudax_platform -c '\dn'

# ── Redis ────────────────────────────────────────────────────────────

.PHONY: redis-shell

redis-shell:  ## Open redis-cli shell
	docker exec -it garudax-redis redis-cli

# ── Kafka ────────────────────────────────────────────────────────────

.PHONY: kafka-topics

kafka-topics:  ## List Kafka topics
	docker exec -it garudax-kafka kafka-topics.sh --bootstrap-server localhost:9092 --list

# ── Pipeline ─────────────────────────────────────────────────────────

.PHONY: pipeline-status pipeline-run pipeline-resume

pipeline-status:  ## Show pipeline task summary
	./pipeline/run.sh --status

pipeline-run:  ## Run pipeline with requirement (use ARGS="Build X")
	./pipeline/run.sh "$(ARGS)"

pipeline-resume:  ## Resume pipeline from tasks.json state
	./pipeline/run.sh --resume

# ── Help ─────────────────────────────────────────────────────────────

.PHONY: help

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'
