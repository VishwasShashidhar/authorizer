package graph

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yauthdev/yauth/server/db"
	"github.com/yauthdev/yauth/server/enum"
	"github.com/yauthdev/yauth/server/graph/generated"
	"github.com/yauthdev/yauth/server/graph/model"
	"github.com/yauthdev/yauth/server/session"
	"github.com/yauthdev/yauth/server/utils"
	"golang.org/x/crypto/bcrypt"
)

func (r *mutationResolver) VerifySignupToken(ctx context.Context, params model.VerifySignupTokenInput) (*model.LoginResponse, error) {
	gc, err := utils.GinContextFromContext(ctx)
	var res *model.LoginResponse
	if err != nil {
		return res, err
	}

	_, err = db.Mgr.GetVerificationByToken(params.Token)
	if err != nil {
		return res, errors.New(`Invalid token`)
	}

	// verify if token exists in db
	claim, err := utils.VerifyVerificationToken(params.Token)
	if err != nil {
		return res, errors.New(`Invalid token`)
	}

	user, err := db.Mgr.GetUserByEmail(claim.Email)
	if err != nil {
		return res, err
	}

	// update email_verified_at in users table
	db.Mgr.UpdateVerificationTime(time.Now().Unix(), user.ID)
	// delete from verification table
	db.Mgr.DeleteToken(claim.Email)

	userIdStr := fmt.Sprintf("%d", user.ID)
	refreshToken, _, _ := utils.CreateAuthToken(utils.UserAuthInfo{
		ID:    userIdStr,
		Email: user.Email,
	}, enum.RefreshToken)

	accessToken, expiresAt, _ := utils.CreateAuthToken(utils.UserAuthInfo{
		ID:    userIdStr,
		Email: user.Email,
	}, enum.AccessToken)

	session.SetToken(userIdStr, refreshToken)

	res = &model.LoginResponse{
		Message:              `Email verified successfully.`,
		AccessToken:          &accessToken,
		AccessTokenExpiresAt: &expiresAt,
		User: &model.User{
			ID:        userIdStr,
			Email:     user.Email,
			Image:     &user.Image,
			FirstName: &user.FirstName,
			LastName:  &user.LastName,
		},
	}

	utils.SetCookie(gc, accessToken)

	return res, nil
}

func (r *mutationResolver) Signup(ctx context.Context, params model.SignUpInput) (*model.SignUpResponse, error) {
	var res *model.SignUpResponse
	if params.CofirmPassword != params.Password {
		return res, errors.New(`Passowrd and Confirm Password does not match`)
	}

	params.Email = strings.ToLower(params.Email)

	if !utils.IsValidEmail(params.Email) {
		return res, errors.New(`Invalid email address`)
	}

	// find user with email
	existingUser, err := db.Mgr.GetUserByEmail(params.Email)
	if err != nil {
		log.Println("User with email " + params.Email + " not found")
	}

	if existingUser.EmailVerifiedAt > 0 {
		// email is verified
		return res, errors.New(`You have already signed up. Please login`)
	}
	user := db.User{
		Email:    params.Email,
		Password: params.Password,
	}

	if params.FirstName != nil {
		user.FirstName = *params.FirstName
	}

	if params.LastName != nil {
		user.LastName = *params.LastName
	}

	user.SignUpMethod = enum.BasicAuth.String()
	_, err = db.Mgr.AddUser(user)
	if err != nil {
		return res, err
	}

	// insert verification request
	verificationType := enum.BasicAuth.String()
	token, err := utils.CreateVerificationToken(params.Email, verificationType)
	if err != nil {
		log.Println(`Error generating token`, err)
	}
	db.Mgr.AddVerification(db.Verification{
		Token:      token,
		Identifier: verificationType,
		ExpiresAt:  time.Now().Add(time.Minute * 30).Unix(),
		Email:      params.Email,
	})

	// exec it as go routin so that we can reduce the api latency
	go func() {
		utils.SendVerificationMail(params.Email, token)
	}()

	res = &model.SignUpResponse{
		Message: `Verification email sent successfully. Please check your inbox`,
	}

	return res, nil
}

func (r *mutationResolver) Login(ctx context.Context, params model.LoginInput) (*model.LoginResponse, error) {
	gc, err := utils.GinContextFromContext(ctx)
	var res *model.LoginResponse
	if err != nil {
		return res, err
	}

	params.Email = strings.ToLower(params.Email)
	user, err := db.Mgr.GetUserByEmail(params.Email)
	if err != nil {
		return res, errors.New(`User with this email not found`)
	}

	if user.SignUpMethod != enum.BasicAuth.String() {
		return res, errors.New(`User has not signed up email & password`)
	}

	if user.EmailVerifiedAt <= 0 {
		return res, errors.New(`Email not verified`)
	}
	// match password
	log.Println("params Pass", params.Password)
	log.Println("hashed pass", user.Password)
	cost, err := bcrypt.Cost([]byte(user.Password))
	log.Println(cost, err)
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(params.Password))

	if err != nil {
		log.Println("Compare password error:", err)
		return res, errors.New(`Invalid Password`)
	}
	userIdStr := fmt.Sprintf("%d", user.ID)
	refreshToken, _, _ := utils.CreateAuthToken(utils.UserAuthInfo{
		ID:    userIdStr,
		Email: user.Email,
	}, enum.RefreshToken)

	accessToken, expiresAt, _ := utils.CreateAuthToken(utils.UserAuthInfo{
		ID:    userIdStr,
		Email: user.Email,
	}, enum.AccessToken)

	session.SetToken(userIdStr, refreshToken)

	res = &model.LoginResponse{
		Message:              `Logged in successfully`,
		AccessToken:          &accessToken,
		AccessTokenExpiresAt: &expiresAt,
		User: &model.User{
			ID:        userIdStr,
			Email:     user.Email,
			Image:     &user.Image,
			FirstName: &user.FirstName,
			LastName:  &user.LastName,
		},
	}

	utils.SetCookie(gc, accessToken)

	return res, nil
}

func (r *mutationResolver) Logout(ctx context.Context) (*model.Response, error) {
	gc, err := utils.GinContextFromContext(ctx)
	var res *model.Response
	if err != nil {
		return res, err
	}

	token, err := utils.GetAuthToken(gc)
	if err != nil {
		return res, err
	}

	claim, err := utils.VerifyAuthToken(token)
	if err != nil {
		return res, err
	}

	session.DeleteToken(claim.ID)
	res = &model.Response{
		Message: "Logged out successfully",
	}

	utils.DeleteCookie(gc)
	return res, nil
}

func (r *queryResolver) Users(ctx context.Context) ([]*model.User, error) {
	var res []*model.User
	users, err := db.Mgr.GetUsers()
	if err != nil {
		return res, err
	}

	for _, user := range users {
		res = append(res, &model.User{
			ID:              fmt.Sprintf("%d", user.ID),
			Email:           user.Email,
			SignUpMethod:    user.SignupMethod,
			FirstName:       &user.FirstName,
			LastName:        &user.LastName,
			Password:        &user.Password,
			EmailVerifiedAt: &user.EmailVerifiedAt,
		})
	}

	return res, nil
}

func (r *queryResolver) Token(ctx context.Context) (*model.LoginResponse, error) {
	gc, err := utils.GinContextFromContext(ctx)
	var res *model.LoginResponse
	if err != nil {
		return res, err
	}
	token, err := utils.GetAuthToken(gc)
	if err != nil {
		return res, err
	}

	claim, accessTokenErr := utils.VerifyAuthToken(token)
	expiresAt := claim.ExpiresAt

	user, err := db.Mgr.GetUserByEmail(claim.Email)
	if err != nil {
		return res, err
	}

	userIdStr := fmt.Sprintf("%d", user.ID)

	sessionToken := session.GetToken(userIdStr)

	if sessionToken == "" {
		return res, errors.New(`Unauthorized`)
	}
	// TODO check if session token has expired

	if accessTokenErr != nil {
		// if access token has expired and refresh/session token is valid
		// generate new accessToken
		fmt.Println(`here... getting new accesstoken`)
		token, expiresAt, _ = utils.CreateAuthToken(utils.UserAuthInfo{
			ID:    userIdStr,
			Email: user.Email,
		}, enum.AccessToken)

	}
	utils.SetCookie(gc, token)
	res = &model.LoginResponse{
		Message:              `Email verified successfully.`,
		AccessToken:          &token,
		AccessTokenExpiresAt: &expiresAt,
		User: &model.User{
			ID:        userIdStr,
			Email:     user.Email,
			Image:     &user.Image,
			FirstName: &user.FirstName,
			LastName:  &user.LastName,
		},
	}
	return res, nil
}

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type (
	mutationResolver struct{ *Resolver }
	queryResolver    struct{ *Resolver }
)
