BEGIN;
SET LOCAL app.tenant_id = 'bf96ef75-40ed-47b8-a8d7-f044fe03c04d';
SELECT COUNT(*) AS my_agents FROM agents;
ROLLBACK;
