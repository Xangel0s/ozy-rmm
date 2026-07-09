-- 005_create_admin_b.sql
-- Creates the admin user for Tenant B with a bcrypt-hashed password.
-- Uses apexrmm (BYPASSRLS) because user creation here is bootstrap.

INSERT INTO users (tenant_id, email, username, password_hash, full_name, role)
VALUES (
  'd5e2fb62-2f70-4ace-bf33-89fefeb52c88',
  'admin-b@apexrmm.com',
  'admin-b',
  '$2a$10$Shabcve//GUzACsIT7TJb.kstS8MVYw2.5Ck8BUiSLvkcthLbi4Ue',
  'Test Admin B',
  'admin'
)
ON CONFLICT (email) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  password_hash = EXCLUDED.password_hash,
  full_name = EXCLUDED.full_name,
  role = EXCLUDED.role
RETURNING id, email, tenant_id, role;
