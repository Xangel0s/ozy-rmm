-- 004_create_tenant_b.sql
-- Creates a second tenant for the cross-tenant isolation test.
-- Bypasses the application role (uses apexrmm directly) because the
-- tenant-creation flow itself isn't part of the test scope.

INSERT INTO tenants (name, slug)
VALUES ('Test Org B', 'test-b')
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
RETURNING id;
