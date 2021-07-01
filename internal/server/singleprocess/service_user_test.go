package singleprocess

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/waypoint/internal/server"
	pb "github.com/hashicorp/waypoint/internal/server/gen"
)

func TestServiceUpdateUser(t *testing.T) {
	ctx := context.Background()

	// Create our server
	impl, err := New(WithDB(testDB(t)))
	require.NoError(t, err)
	client := server.TestServer(t, impl)

	// Get ourself
	userResp, err := client.GetUser(ctx, &pb.GetUserRequest{})
	require.NoError(t, err)
	user := userResp.User

	// Simplify writing tests
	type Req = pb.UpdateUserRequest

	t.Run("update but do nothing", func(t *testing.T) {
		require := require.New(t)

		resp, err := client.UpdateUser(ctx, &Req{User: user})
		require.NoError(err)
		require.NotNil(resp)
		require.Equal(user.Id, resp.User.Id)
	})

	t.Run("update without a user set", func(t *testing.T) {
		require := require.New(t)

		// Create, should get an ID back
		_, err := client.UpdateUser(ctx, &Req{})
		require.Error(err)
	})

	t.Run("update with an invalid ID", func(t *testing.T) {
		require := require.New(t)

		userCopy := *user
		userCopy.Id = "NOPE"

		_, err := client.UpdateUser(ctx, &Req{User: &userCopy})
		require.Error(err)
	})

	t.Run("change username", func(t *testing.T) {
		require := require.New(t)

		userCopy := *user
		userCopy.Username = "tubes"

		resp, err := client.UpdateUser(ctx, &Req{User: &userCopy})
		require.NoError(err)
		require.NotNil(resp)
		require.Equal(user.Id, resp.User.Id)
		require.Equal("tubes", resp.User.Username)
	})
}
