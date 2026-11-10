package users

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func newMockManager(t *testing.T) (Manager, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return Manager{db: db}, mock
}

func TestCreateOperatorUser(t *testing.T) {
	m, mock := newMockManager(t)

	// Operator-managed accounts must never expire: the operator rotates this
	// password itself and does not expect the server to force a reset.
	mock.ExpectExec("CREATE USER IF NOT EXISTS 'operator'@'%' IDENTIFIED BY ? PASSWORD EXPIRE NEVER").
		WithArgs("pass1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("GRANT ALL ON *.* TO 'operator'@'%' WITH GRANT OPTION").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, m.CreateOperatorUser("pass1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUserPass(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectBegin()
	// Dual-password rotation must also disable password_history /
	// password_reuse_interval, otherwise re-applying a previously used
	// password is rejected by the server as a reuse violation.
	mock.ExpectExec("ALTER USER ?@? IDENTIFIED BY ? RETAIN CURRENT PASSWORD PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY").
		WithArgs("test", "host1", "newpass").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := m.UpdateUserPass(&SysUser{Name: "test", Pass: "newpass", Hosts: []string{"host1"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUserPass_NilUser(t *testing.T) {
	m, mock := newMockManager(t)

	require.NoError(t, m.UpdateUserPass(nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUserPass_RollbackOnError(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectBegin()
	mock.ExpectExec("ALTER USER ?@? IDENTIFIED BY ? RETAIN CURRENT PASSWORD PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY").
		WithArgs("test", "host1", "newpass").
		WillReturnError(assertError{"boom"})
	mock.ExpectRollback()

	err := m.UpdateUserPass(&SysUser{Name: "test", Pass: "newpass", Hosts: []string{"host1"}})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisablePasswordPolicies(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectExec("ALTER USER ?@? PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY PASSWORD REQUIRE CURRENT OPTIONAL").
		WithArgs("test", "host1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ALTER USER ?@? PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY PASSWORD REQUIRE CURRENT OPTIONAL").
		WithArgs("test", "host2").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := m.DisablePasswordPolicies(&SysUser{Name: "test", Hosts: []string{"host1", "host2"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDisablePasswordPolicies_NilUser(t *testing.T) {
	m, mock := newMockManager(t)

	require.NoError(t, m.DisablePasswordPolicies(nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdate160MonitorUserGrant(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectExec("CREATE USER IF NOT EXISTS 'monitor'@'%' IDENTIFIED BY ? PASSWORD EXPIRE NEVER").
		WithArgs("pass1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("/*!80015 GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'%' */").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("ALTER USER 'monitor'@'%' WITH MAX_USER_CONNECTIONS 100").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, m.Update160MonitorUserGrant("pass1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdate1150XtrabackupUser(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectExec("CREATE USER IF NOT EXISTS 'xtrabackup'@'%' IDENTIFIED BY ? PASSWORD EXPIRE NEVER").
		WithArgs("pass1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("GRANT ALL ON *.* TO 'xtrabackup'@'%' WITH GRANT OPTION").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, m.Update1150XtrabackupUser("pass1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateReplicationUser(t *testing.T) {
	m, mock := newMockManager(t)

	mock.ExpectExec("CREATE USER IF NOT EXISTS 'replication'@'%' IDENTIFIED BY ? PASSWORD EXPIRE NEVER").
		WithArgs("pass1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("GRANT REPLICATION SLAVE ON *.* to 'replication'@'%'").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, m.CreateReplicationUser("pass1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// assertError is a minimal error implementation used to force a driver error
// without pulling in extra dependencies.
type assertError struct{ msg string }

func (e assertError) Error() string { return e.msg }
