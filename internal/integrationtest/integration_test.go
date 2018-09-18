package integrationtest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	proto1 "github.com/gogo/protobuf/proto"
	"github.com/kylelemons/godebug/pretty"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal/proto"
	"github.com/samsarahq/thunder/internal/testfixtures"
	"github.com/samsarahq/thunder/sqlgen"
	"github.com/stretchr/testify/require"
)

func setupDB() (*testfixtures.TestDatabase, *sqlgen.DB, error) {
	testDb, err := testfixtures.NewTestDatabase()
	if err != nil {
		return nil, nil, err
	}

	if _, err = testDb.Exec(`
		CREATE TABLE complex_proto (
			id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			regular_string VARCHAR(255),
			zero_null_string VARCHAR(255),
			regular_int64 BIGINT,
			zero_null_int64 BIGINT,
			no_graph_string VARCHAR(255)
		)
	`); err != nil {
		return nil, nil, err
	}
	schema := sqlgen.NewSchema()
	schema.MustRegisterType("complex_proto", sqlgen.AutoIncrement, proto.ComplexProto{})

	return testDb, sqlgen.NewDB(testDb.DB, schema), nil
}

// #1 Support ZeroIsNull conditions for GQL and the DB.
func TestTypeConversions(t *testing.T) {
	tdb, db, err := setupDB()
	require.NoError(t, err)
	defer tdb.Close()
	ctx := context.Background()
	// Test should take an expected proto type, and ensure that conversions to the
	// db and back keep the type equivalent.  Same for GQL requests.
	protoType := &proto.ComplexProto{
		Id:             1,
		RegularString:  "abcd",
		ZeroNullString: "abcd",
		RegularInt64:   5,
		ZeroNullInt64:  1,
	}

	mshl, err := proto1.Marshal(protoType)
	require.NoError(t, err)

	newType := &proto.ComplexProto{}
	err = proto1.Unmarshal(mshl, newType)
	require.NoError(t, err)
	require.Equal(t, protoType, newType)

	_, err = db.InsertRow(ctx, protoType)
	require.NoError(t, err)

	dbRow := &proto.ComplexProto{}
	require.NoError(t, db.QueryRow(ctx, &dbRow, nil, nil))

	require.Equal(t, protoType, dbRow)

	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("proto", func() *proto.ComplexProto {
		return protoType
	})
	schema.Object("proto", proto.ComplexProto{})
	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			proto { id, regularString}
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	res, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	wantResult := `{ "proto": { "id": 1 } }`
	var expected interface{}
	require.NoError(t, json.Unmarshal([]byte(wantResult), &expected))
	diff := pretty.Compare(
		res,
		expected,
	)
	require.Equal(t, diff, "", "there was a difference in the expected GQL")
	fmt.Println(res)
	// Null/ZeroValue DB Fields vs Non-null DB fields.
	// Custom types that have Nullness embedded in the field itself.
	// Null/Empty/ZeroValue GQL fields vs Non-null Fields


	obj := proto.ComplexProto{}
	ConversionTest(
		SerializeAndDeserialize(),
		StoreAndReadFromDB(
			AssertDBFieldIsNull("field"),
			AssertDBFieldIsEqual("field", "wantValue"),
		),
		AssertOnValue(),
		QueryGQL("...", map[string]...wantResult),
		QueryGQLAndExpectError("...", map[string]...wantResult)
	)
}
