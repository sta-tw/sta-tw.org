ALTER TABLE account_roles DROP CONSTRAINT account_roles_role_check;
ALTER TABLE account_roles ADD CONSTRAINT account_roles_role_check
    CHECK (role IN ('admin', 'ai_system', 'service', 'admissions_moderator'));

COMMENT ON TABLE account_roles IS
    '角色清單：admin（完整管理員）、ai_system、service（機器帳號）、'
    'admissions_moderator（僅能管理簡章／招生資料，看不到其他後台頁面）。';
