package grpcadapter

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

var errCardsNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("student cards are not served"))

// GetCardCode returns the code a pupil's card shows (task 13).
func (h *StudentsHandler) GetCardCode(ctx context.Context, req *connect.Request[studentsv1.GetCardCodeRequest]) (*connect.Response[studentsv1.GetCardCodeResponse], error) {
	if h.cards == nil {
		return nil, errCardsNotServed
	}
	got, err := h.cards.CardCode(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetCardCodeResponse{Code: got.Code, Suspended: got.Suspended}), nil
}

// ReissueCard gives a suspended pupil a new card.
func (h *StudentsHandler) ReissueCard(ctx context.Context, req *connect.Request[studentsv1.ReissueCardRequest]) (*connect.Response[studentsv1.ReissueCardResponse], error) {
	if h.cards == nil {
		return nil, errCardsNotServed
	}
	if err := h.cards.ReissueCard(ctx, req.Msg.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ReissueCardResponse{}), nil
}

// RevokeCard retires a pupil's current code.
func (h *StudentsHandler) RevokeCard(ctx context.Context, req *connect.Request[studentsv1.RevokeCardRequest]) (*connect.Response[studentsv1.RevokeCardResponse], error) {
	if h.cards == nil {
		return nil, errCardsNotServed
	}
	if err := h.cards.RevokeCard(ctx, req.Msg.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.RevokeCardResponse{}), nil
}

// VerifyCard reads a scanned code: whose card it is, and whether it counts.
func (h *StudentsHandler) VerifyCard(ctx context.Context, req *connect.Request[studentsv1.VerifyCardRequest]) (*connect.Response[studentsv1.VerifyCardResponse], error) {
	if h.cards == nil {
		return nil, errCardsNotServed
	}
	got, err := h.cards.VerifyCard(ctx, req.Msg.GetCode())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.VerifyCardResponse{Student: toProtoStudent(got.Student), Valid: got.Valid}), nil
}
