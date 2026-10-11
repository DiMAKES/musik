"""Identity and profile ownership: the boundary between the shared catalog and
personal data, made explicit for both engines.

The baseline adopts the SQLite history (steps 11 and 12 add users, profiles
and profile_id on personal tables) or creates the same schema on PostgreSQL
from its reflected metadata; this revision checks the result and, on a new
PostgreSQL database, seeds the installation owner and its main profile.
"""
from datetime import datetime, timezone
from uuid import uuid4

from alembic import op
from sqlalchemy import inspect, text


revision = "musik_12"
down_revision = "musik_5"
branch_labels = None
depends_on = None


def upgrade() -> None:
    connection = op.get_bind()
    missing = {"users", "profiles", "external_identities", "auth_sessions", "invitations"} - set(
        inspect(connection).get_table_names()
    )
    if missing:
        raise RuntimeError(f"identity schema is incomplete: {', '.join(sorted(missing))}")
    if connection.dialect.name == "sqlite":
        version = connection.exec_driver_sql("PRAGMA user_version").scalar_one()
        if version < 12:
            raise RuntimeError("SQLite identity migration did not reach schema version 12")
    tables = set(inspect(connection).get_table_names())
    required = {"installation_state", "listening_history", "playlists", "recommendation_impressions"}
    missing = required - tables
    if missing:
        raise RuntimeError(f"profile ownership schema is incomplete: {', '.join(sorted(missing))}")
    for table in ("listening_history", "playlists", "recommendation_impressions"):
        columns = {column["name"] for column in inspect(connection).get_columns(table)}
        if "profile_id" not in columns:
            raise RuntimeError(f"profile ownership is missing {table}.profile_id")
    if connection.dialect.name == "postgresql":
        seeded = connection.execute(
            text("SELECT 1 FROM installation_state WHERE key='legacy_profile_id'")
        ).first()
        if seeded is None:
            now = datetime.now(timezone.utc).isoformat()
            user_id, profile_id = str(uuid4()), str(uuid4())
            connection.execute(
                text("INSERT INTO users(id,status,display_name,created_at,updated_at) VALUES (:id,'disabled','Installation owner',:now,:now)"),
                {"id": user_id, "now": now},
            )
            connection.execute(text("INSERT INTO user_roles(user_id,role) VALUES (:id,'admin'),(:id,'user')"), {"id": user_id})
            connection.execute(
                text("INSERT INTO profiles(id,owner_user_id,name,is_default,created_at,updated_at) VALUES (:profile,:owner,'Main',1,:now,:now)"),
                {"profile": profile_id, "owner": user_id, "now": now},
            )
            connection.execute(text("INSERT INTO installation_state(key,value) VALUES ('legacy_user_id',:user),('legacy_profile_id',:profile)"), {"user": user_id, "profile": profile_id})


def downgrade() -> None:
    raise RuntimeError("musik data migrations are forward-only; restore a verified backup to roll back")
