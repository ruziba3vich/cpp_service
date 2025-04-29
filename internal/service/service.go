package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ruziba3vich/cpp_service/genprotos/genprotos/compiler_service"
	"github.com/ruziba3vich/cpp_service/internal/storage"
	"github.com/ruziba3vich/cpp_service/pkg/config"
	logger "github.com/ruziba3vich/prodonik_lgger"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type CppExecutorServer struct {
	compiler_service.UnimplementedCodeExecutorServer
	clients map[string]*storage.CppClient
	mu      sync.Mutex
	logger  *logger.Logger
	cfg     *config.Config
}

func NewCppExecutorServer(cfg *config.Config, logger *logger.Logger) *CppExecutorServer {
	return &CppExecutorServer{
		clients: make(map[string]*storage.CppClient),
		logger:  logger,
		cfg:     cfg,
	}
}

func (s *CppExecutorServer) removeClient(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, exists := s.clients[sessionID]; exists {
		s.logger.Info("Removing C++ client session", map[string]any{"session_id": sessionID})
		client.Cleanup()
		delete(s.clients, sessionID)
		s.logger.Info("C++ client session removed", map[string]any{"session_id": sessionID})
	} else {
		s.logger.Warn("C++ client session to remove not found (already removed?)", map[string]any{"session_id": sessionID})
	}
}

// Execute handles the bidirectional gRPC stream for C++ code execution
func (s *CppExecutorServer) Execute(stream compiler_service.CodeExecutor_ExecuteServer) error {
	sessionID := ""
	var client *storage.CppClient
	var clientAddr string

	if p, ok := peer.FromContext(stream.Context()); ok {
		clientAddr = p.Addr.String()
	}
	s.logger.Info("New C++ Execute stream started", map[string]any{"client_addr": clientAddr})

	defer func() {
		if sessionID != "" {
			s.logger.Info("C++ Execute stream ended. Initiating cleanup for session",
				map[string]any{"session_id": sessionID})
			s.removeClient(sessionID)
		} else {
			s.logger.Info("C++ Execute stream ended before session was established",
				map[string]any{"client_addr": clientAddr})
		}
	}()

	for {
		req, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.logger.Info("Stream closed by client (EOF)", map[string]any{"session_id": sessionID, "client_addr": clientAddr})
				return nil
			}
			st, _ := status.FromError(err)
			if st.Code() == codes.Canceled || st.Code() == codes.DeadlineExceeded {
				s.logger.Warn("Stream context cancelled or deadline exceeded",
					map[string]any{"session_id": sessionID, "error": err.Error(), "code": st.Code()})
				return err
			}
			s.logger.Error(fmt.Sprintf("Stream receive error: %v", err),
				map[string]any{"session_id": sessionID, "error": err.Error()})
			return status.Errorf(codes.Internal, "stream receive error: %v", err)
		}

		if client == nil {
			sessionID = req.SessionId
			if sessionID == "" {
				sessionID = uuid.NewString()
				s.logger.Warn("No session_id provided by client, generated new one",
					map[string]any{"client_addr": clientAddr, "generated_session_id": sessionID})
			}
			s.logger.Info("Initial request received, establishing session", map[string]any{"session_id": sessionID, "client_addr": clientAddr})

			s.mu.Lock()

			if existingClient, exists := s.clients[sessionID]; exists {
				if existingClient.CtxDone() {
					s.logger.Warn("Stale session found, cleaning up previous instance before creating new one",
						map[string]any{"session_id": sessionID})
					s.mu.Unlock()
					s.removeClient(sessionID)
					s.mu.Lock()
					if _, stillExists := s.clients[sessionID]; stillExists {
						s.mu.Unlock()
						s.logger.Error("Session race condition detected after cleaning stale C++ session", map[string]any{"session_id": sessionID})
						return status.Errorf(codes.Aborted, "session creation race condition for %s", sessionID)
					}
				} else {
					s.mu.Unlock()
					s.logger.Error("Attempted to create an already active C++ session", map[string]any{"session_id": sessionID})
					return status.Errorf(codes.AlreadyExists, "session %s is already active", sessionID)
				}
			}

			clientCtx, clientCancel := context.WithTimeout(context.Background(), 90*time.Second)
			linkedCtx, linkedCancel := context.WithCancel(stream.Context())

			go func(sessID string) {
				<-linkedCtx.Done()
				s.logger.Info(fmt.Sprintf("Stream context done (%v), cancelling client context for C++ session",
					linkedCtx.Err()), map[string]any{"session_id": sessID})
				clientCancel()
				linkedCancel()
			}(sessionID)

			client = storage.NewCppClient(sessionID, clientCtx, clientCancel, s.cfg, s.logger)
			s.clients[sessionID] = client
			s.logger.Info("New C++ client created and stored", map[string]any{"session_id": sessionID})
			s.mu.Unlock()

			go client.SendResponses(stream)

		} else {
			if req.SessionId != sessionID {
				s.logger.Warn(fmt.Sprintf("Received message with mismatched session ID: expected %s, got %s", sessionID, req.SessionId),
					map[string]any{"session_id": sessionID, "received_session_id": req.SessionId})
				client.SendResponse(&compiler_service.ExecuteResponse{
					SessionId: sessionID,
					Payload: &compiler_service.ExecuteResponse_Error{
						Error: &compiler_service.Error{
							ErrorText: fmt.Sprintf("Mismatched session ID: expected %s, got %s", sessionID, req.SessionId),
						},
					},
				})
				continue
			}
		}

		switch payload := req.Payload.(type) {
		case *compiler_service.ExecuteRequest_Code:
			s.logger.Info("Received Code payload for C++ execution", map[string]any{"session_id": sessionID})
			if payload.Code.Language != "cpp" {
				s.logger.Warn(fmt.Sprintf("Unsupported language received: %s, expected 'cpp'", payload.Code.Language),
					map[string]any{"session_id": sessionID, "language": payload.Code.Language})
				client.SendResponse(&compiler_service.ExecuteResponse{
					SessionId: sessionID,
					Payload: &compiler_service.ExecuteResponse_Error{
						Error: &compiler_service.Error{ErrorText: fmt.Sprintf("Unsupported language '%s'. This service only supports 'cpp'.", payload.Code.Language)}, // Update error message
					},
				})
				continue
			}
			s.logger.Info("Starting C++ execution process", map[string]any{"session_id": sessionID})
			go client.ExecuteCpp(payload.Code.SourceCode)

		case *compiler_service.ExecuteRequest_Input:
			s.logger.Info(fmt.Sprintf("Received Input payload: %q", payload.Input.InputText),
				map[string]any{"session_id": sessionID, "input_length": len(payload.Input.InputText)})
			client.HandleInput(payload.Input.InputText)

		default:
			s.logger.Warn(fmt.Sprintf("Received unknown payload type: %T", payload),
				map[string]any{"session_id": sessionID, "payload_type": fmt.Sprintf("%T", payload)})
			client.SendResponse(&compiler_service.ExecuteResponse{
				SessionId: sessionID,
				Payload: &compiler_service.ExecuteResponse_Error{
					Error: &compiler_service.Error{ErrorText: "Unknown or unsupported request payload type"},
				},
			})
		}
	}
}
