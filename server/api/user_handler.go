package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/go-chi/chi"
	"github.com/porter-dev/porter/internal/forms"
	"github.com/porter-dev/porter/internal/models"
	"github.com/porter-dev/porter/internal/repository"
)

// Enumeration of user API error codes, represented as int64
const (
	ErrUserDecode ErrorCode = iota + 600
	ErrUserValidateFields
	ErrUserDataWrite
	ErrUserDataRead
)

// HandleCreateUser validates a user form entry, converts the user to a gorm
// model, and saves the user to the database
func (app *App) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	form := &forms.CreateUserForm{}

	user, err := app.writeUser(
		form,
		app.repo.User.CreateUser,
		w,
		r,
		doesUserExist,
	)

	if err == nil {
		app.logger.Info().Msgf("New user created: %d", user.ID)
		w.WriteHeader(http.StatusCreated)
	}
}

// HandleReadUser returns an externalized User (models.UserExternal)
// based on an ID
func (app *App) HandleReadUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 0, 64)

	if err != nil || id == 0 {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return
	}

	user, err := app.repo.User.ReadUser(uint(id))

	if err != nil {
		app.handleErrorRead(err, ErrUserDataRead, w)
		return
	}

	extUser := user.Externalize()

	if err := json.NewEncoder(w).Encode(extUser); err != nil {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleUpdateUser validates an update user form entry, updates the user
// in the database, and writes status accepted
func (app *App) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 0, 64)

	if err != nil || id == 0 {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return
	}

	form := &forms.UpdateUserForm{
		ID: uint(id),
	}

	user, err := app.writeUser(form, app.repo.User.UpdateUser, w, r)

	if err == nil {
		app.logger.Info().Msgf("User updated: %d", user.ID)
		w.WriteHeader(http.StatusAccepted)
	}
}

// HandleDeleteUser is majestic
func (app *App) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 0, 64)

	if err != nil || id == 0 {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return
	}

	// TODO -- HASH AND VERIFY PASSWORD BEFORE USER DELETION
	form := &forms.DeleteUserForm{
		ID:       uint(id),
		Password: "testing",
	}

	user, err := app.writeUser(form, app.repo.User.DeleteUser, w, r)

	if err == nil {
		app.logger.Info().Msgf("User deleted: %d", user.ID)
		w.WriteHeader(http.StatusAccepted)
	}
}

// ------------------------ User handler helper functions ------------------------ //

// writeUser will take a POST or PUT request to the /api/users endpoint and decode
// the request into a forms.WriteUserForm model, convert it to a models.User, and
// write to the database.
func (app *App) writeUser(
	form forms.WriteUserForm,
	dbWrite repository.WriteUser,
	w http.ResponseWriter,
	r *http.Request,
	validators ...func(repo *repository.Repository, user *models.User) *HTTPError,
) (*models.User, error) {
	// decode from JSON to form value
	if err := json.NewDecoder(r.Body).Decode(form); err != nil {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return nil, err
	}

	// validate the form
	if err := app.validator.Struct(form); err != nil {
		app.handleErrorFormValidation(err, ErrUserValidateFields, w)
		return nil, err
	}

	// convert the form to a user model -- WriteUserForm must implement ToUser
	userModel, err := form.ToUser()

	if err != nil {
		app.handleErrorFormDecoding(err, ErrUserDecode, w)
		return nil, err
	}

	// Check any additional validators for any semantic errors
	// We have completed all syntax checks, so these will be sent
	// with http.StatusUnprocessableEntity (422), unless this is
	// an internal server error
	for _, validator := range validators {
		err := validator(app.repo, userModel)

		if err != nil {
			goErr := errors.New(strings.Join(err.Errors, ", "))
			if err.Code == 500 {
				app.sendExternalError(
					goErr,
					http.StatusInternalServerError,
					*err,
					w,
				)
			} else {
				app.sendExternalError(
					goErr,
					http.StatusUnprocessableEntity,
					*err,
					w,
				)
			}

			return nil, goErr
		}
	}

	// handle write to the database
	user, err := dbWrite(userModel)

	if err != nil {
		app.handleErrorDataWrite(err, ErrUserDataWrite, w)
		return nil, err
	}

	return user, nil
}

func doesUserExist(repo *repository.Repository, user *models.User) *HTTPError {
	user, err := repo.User.ReadUserByEmail(user.Email)

	if user != nil && err == nil {
		return &HTTPError{
			Code: ErrUserValidateFields,
			Errors: []string{
				"Email already taken",
			},
		}
	}

	if err != gorm.ErrRecordNotFound {
		return &ErrorDataRead
	}

	return nil
}
