package demo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/vyuha/vyuha-ai/internal/graph"
)

// storageWriter is the minimal interface needed to persist demo data.
type storageWriter interface {
	SaveNodes(ctx context.Context, nodes []*graph.Node) error
	SaveEdges(ctx context.Context, edges []*graph.Edge) error
	GetAllNodes(ctx context.Context) ([]*graph.Node, error)
	GetAllEdges(ctx context.Context) ([]*graph.Edge, error)
}

// SeedDemoGraph creates a small but representative code graph for demonstration.
// It builds a realistic Go microservice structure with services, packages, files,
// functions, structs, interfaces, and cloud dependencies.
func SeedDemoGraph(ctx context.Context, store storageWriter, index *graph.GraphIndex) error {
	nodes := makeDemoNodes()
	edges := makeDemoEdges()

	if err := store.SaveNodes(ctx, nodes); err != nil {
		return fmt.Errorf("demo seed save nodes: %w", err)
	}
	if err := store.SaveEdges(ctx, edges); err != nil {
		return fmt.Errorf("demo seed save edges: %w", err)
	}
	if err := index.LoadFromStorage(ctx, store); err != nil {
		return fmt.Errorf("demo seed reload index: %w", err)
	}
	return nil
}

func makeDemoNodes() []*graph.Node {
	return []*graph.Node{
		// Repository root
		{ID: "repo:demo/microservice", Type: graph.NodeTypeRepository, Name: "demo-microservice", Depth: 0, Metadata: graph.NodeMetadata{}},

		// Services
		{ID: "service:demo/microservice/cmd/api", Type: graph.NodeTypeService, Name: "api-server", ParentID: "repo:demo/microservice", Depth: 1, IsExported: true, Metadata: graph.NodeMetadata{
			EntryPoint: "main", FunctionCount: 12, PackageCount: 4,
		}},
		{ID: "service:demo/microservice/cmd/worker", Type: graph.NodeTypeService, Name: "background-worker", ParentID: "repo:demo/microservice", Depth: 1, IsExported: true, Metadata: graph.NodeMetadata{
			EntryPoint: "main", FunctionCount: 8, PackageCount: 3,
		}},

		// Packages — API service
		{ID: "pkg:demo/microservice/internal/handler", Type: graph.NodeTypePackage, Name: "handler", ParentID: "service:demo/microservice/cmd/api", Depth: 2, Metadata: graph.NodeMetadata{
			FunctionCount: 6,
		}},
		{ID: "pkg:demo/microservice/internal/service", Type: graph.NodeTypePackage, Name: "service", ParentID: "service:demo/microservice/cmd/api", Depth: 2, Metadata: graph.NodeMetadata{
			FunctionCount: 4,
		}},
		{ID: "pkg:demo/microservice/internal/repository", Type: graph.NodeTypePackage, Name: "repository", ParentID: "service:demo/microservice/cmd/api", Depth: 2, Metadata: graph.NodeMetadata{
			FunctionCount: 5,
		}},
		{ID: "pkg:demo/microservice/internal/model", Type: graph.NodeTypePackage, Name: "model", ParentID: "service:demo/microservice/cmd/api", Depth: 2, Metadata: graph.NodeMetadata{
			FunctionCount: 2,
		}},

		// Files
		{ID: "file:demo/microservice/internal/handler/user.go", Type: graph.NodeTypeFile, Name: "user.go", ParentID: "pkg:demo/microservice/internal/handler", FilePath: "internal/handler/user.go", Depth: 3, Metadata: graph.NodeMetadata{
			PackageName: "handler", FunctionCount: 4,
		}},
		{ID: "file:demo/microservice/internal/service/user_service.go", Type: graph.NodeTypeFile, Name: "user_service.go", ParentID: "pkg:demo/microservice/internal/service", FilePath: "internal/service/user_service.go", Depth: 3, Metadata: graph.NodeMetadata{
			PackageName: "service", FunctionCount: 4,
		}},
		{ID: "file:demo/microservice/internal/repository/postgres.go", Type: graph.NodeTypeFile, Name: "postgres.go", ParentID: "pkg:demo/microservice/internal/repository", FilePath: "internal/repository/postgres.go", Depth: 3, Metadata: graph.NodeMetadata{
			PackageName: "repository", FunctionCount: 5,
		}},

		// Functions — handler layer
		{ID: "func:demo/microservice/internal/handler:HandleCreateUser", Type: graph.NodeTypeFunction, Name: "HandleCreateUser", ParentID: "file:demo/microservice/internal/handler/user.go", FilePath: "internal/handler/user.go", LineStart: 25, LineEnd: 68, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (h *UserHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request)", Receiver: "UserHandler", CyclomaticComplexity: 6, HasErrorReturn: true, HasContextParam: true,
			SourceSnippet: "func (h *UserHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {\n\tctx := r.Context()\n\tvar req CreateUserRequest\n\tif err := json.NewDecoder(r.Body).Decode(&req); err != nil {\n\t\thttp.Error(w, err.Error(), 400)\n\t\treturn\n\t}\n\tuser, err := h.svc.CreateUser(ctx, req)\n\t// ...\n}",
		}},
		{ID: "func:demo/microservice/internal/handler:HandleGetUser", Type: graph.NodeTypeFunction, Name: "HandleGetUser", ParentID: "file:demo/microservice/internal/handler/user.go", FilePath: "internal/handler/user.go", LineStart: 70, LineEnd: 95, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (h *UserHandler) HandleGetUser(w http.ResponseWriter, r *http.Request)", Receiver: "UserHandler", CyclomaticComplexity: 3, HasErrorReturn: true, HasContextParam: true,
		}},
		{ID: "func:demo/microservice/internal/handler:HandleListUsers", Type: graph.NodeTypeFunction, Name: "HandleListUsers", ParentID: "file:demo/microservice/internal/handler/user.go", FilePath: "internal/handler/user.go", LineStart: 97, LineEnd: 140, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (h *UserHandler) HandleListUsers(w http.ResponseWriter, r *http.Request)", Receiver: "UserHandler", CyclomaticComplexity: 8, HasErrorReturn: true, HasContextParam: true,
		}},
		{ID: "func:demo/microservice/internal/handler:HandleDeleteUser", Type: graph.NodeTypeFunction, Name: "HandleDeleteUser", ParentID: "file:demo/microservice/internal/handler/user.go", FilePath: "internal/handler/user.go", LineStart: 142, LineEnd: 185, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (h *UserHandler) HandleDeleteUser(w http.ResponseWriter, r *http.Request)", Receiver: "UserHandler", CyclomaticComplexity: 4, HasErrorReturn: true, HasContextParam: true,
		}},

		// Functions — service layer
		{ID: "func:demo/microservice/internal/service:CreateUser", Type: graph.NodeTypeFunction, Name: "CreateUser", ParentID: "file:demo/microservice/internal/service/user_service.go", FilePath: "internal/service/user_service.go", LineStart: 30, LineEnd: 85, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error)", Receiver: "UserService", CyclomaticComplexity: 12, HasErrorReturn: true, HasContextParam: true,
		}},
		{ID: "func:demo/microservice/internal/service:ValidateUser", Type: graph.NodeTypeFunction, Name: "ValidateUser", ParentID: "file:demo/microservice/internal/service/user_service.go", FilePath: "internal/service/user_service.go", LineStart: 87, LineEnd: 130, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (s *UserService) ValidateUser(user *User) error", Receiver: "UserService", CyclomaticComplexity: 22, HasErrorReturn: true, HasContextParam: false,
		}},

		// Functions — repository layer
		{ID: "func:demo/microservice/internal/repository:InsertUser", Type: graph.NodeTypeFunction, Name: "InsertUser", ParentID: "file:demo/microservice/internal/repository/postgres.go", FilePath: "internal/repository/postgres.go", LineStart: 45, LineEnd: 90, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (r *PostgresRepo) InsertUser(ctx context.Context, user *User) error", Receiver: "PostgresRepo", CyclomaticComplexity: 5, HasErrorReturn: true, HasContextParam: true,
		}},
		{ID: "func:demo/microservice/internal/repository:FindUserByID", Type: graph.NodeTypeFunction, Name: "FindUserByID", ParentID: "file:demo/microservice/internal/repository/postgres.go", FilePath: "internal/repository/postgres.go", LineStart: 92, LineEnd: 120, Depth: 4, IsExported: true, Metadata: graph.NodeMetadata{
			Signature: "func (r *PostgresRepo) FindUserByID(ctx context.Context, id string) (*User, error)", Receiver: "PostgresRepo", CyclomaticComplexity: 3, HasErrorReturn: true, HasContextParam: true,
		}},

		// Structs
		{ID: "struct:demo/microservice/internal/model:User", Type: graph.NodeTypeStruct, Name: "User", ParentID: "pkg:demo/microservice/internal/model", FilePath: "internal/model/user.go", Depth: 3, IsExported: true, Metadata: graph.NodeMetadata{
			Fields: []graph.StructField{
				{Name: "ID", Type: "string", IsExported: true},
				{Name: "Email", Type: "string", IsExported: true},
				{Name: "Name", Type: "string", IsExported: true},
				{Name: "CreatedAt", Type: "time.Time", IsExported: true},
				{Name: "UpdatedAt", Type: "time.Time", IsExported: true},
			},
			ImplementsInterfaces: []string{"interface:demo/microservice/internal/model:Validatable"},
		}},
		{ID: "struct:demo/microservice/internal/handler:CreateUserRequest", Type: graph.NodeTypeStruct, Name: "CreateUserRequest", ParentID: "pkg:demo/microservice/internal/handler", FilePath: "internal/handler/user.go", Depth: 3, IsExported: true, Metadata: graph.NodeMetadata{
			Fields: []graph.StructField{
				{Name: "Email", Type: "string", IsExported: true},
				{Name: "Name", Type: "string", IsExported: true},
				{Name: "Password", Type: "string", IsExported: true},
			},
		}},

		// Interfaces
		{ID: "interface:demo/microservice/internal/service:UserRepository", Type: graph.NodeTypeInterface, Name: "UserRepository", ParentID: "pkg:demo/microservice/internal/service", FilePath: "internal/service/user_service.go", Depth: 3, IsExported: true, Metadata: graph.NodeMetadata{
			InterfaceMethods: []graph.InterfaceMethod{
				{Name: "InsertUser", Signature: "InsertUser(ctx context.Context, user *User) error"},
				{Name: "FindUserByID", Signature: "FindUserByID(ctx context.Context, id string) (*User, error)"},
				{Name: "DeleteUser", Signature: "DeleteUser(ctx context.Context, id string) error"},
			},
			Implementors: []string{"struct:demo/microservice/internal/repository:PostgresRepo"},
		}},
		{ID: "interface:demo/microservice/internal/model:Validatable", Type: graph.NodeTypeInterface, Name: "Validatable", ParentID: "pkg:demo/microservice/internal/model", FilePath: "internal/model/validate.go", Depth: 3, IsExported: true, Metadata: graph.NodeMetadata{
			InterfaceMethods: []graph.InterfaceMethod{
				{Name: "Validate", Signature: "Validate() error"},
			},
			Implementors: []string{"struct:demo/microservice/internal/model:User"},
		}},

		// Cloud services
		{ID: "cloud:aws:rds", Type: graph.NodeTypeCloudService, Name: "AWS RDS (PostgreSQL)", ParentID: "repo:demo/microservice", Depth: 1, Metadata: graph.NodeMetadata{
			Provider: "aws", Service: "rds", DetectedOperations: []string{"Query", "Exec", "Prepare"},
		}},
		{ID: "cloud:aws:sqs", Type: graph.NodeTypeCloudService, Name: "AWS SQS", ParentID: "repo:demo/microservice", Depth: 1, Metadata: graph.NodeMetadata{
			Provider: "aws", Service: "sqs", DetectedOperations: []string{"SendMessage", "ReceiveMessage"},
		}},
		{ID: "cloud:aws:s3", Type: graph.NodeTypeCloudService, Name: "AWS S3", ParentID: "repo:demo/microservice", Depth: 1, Metadata: graph.NodeMetadata{
			Provider: "aws", Service: "s3", DetectedOperations: []string{"PutObject", "GetObject"},
		}},
	}
}

func makeDemoEdges() []*graph.Edge {
	e := func(src, tgt string, etype graph.EdgeType) *graph.Edge {
		return &graph.Edge{ID: uuid.NewString(), SourceID: src, TargetID: tgt, Type: etype, Metadata: graph.EdgeMetadata{}}
	}
	return []*graph.Edge{
		// Containment hierarchy
		e("repo:demo/microservice", "service:demo/microservice/cmd/api", graph.EdgeTypeContains),
		e("repo:demo/microservice", "service:demo/microservice/cmd/worker", graph.EdgeTypeContains),
		e("service:demo/microservice/cmd/api", "pkg:demo/microservice/internal/handler", graph.EdgeTypeContains),
		e("service:demo/microservice/cmd/api", "pkg:demo/microservice/internal/service", graph.EdgeTypeContains),
		e("service:demo/microservice/cmd/api", "pkg:demo/microservice/internal/repository", graph.EdgeTypeContains),
		e("service:demo/microservice/cmd/api", "pkg:demo/microservice/internal/model", graph.EdgeTypeContains),
		e("pkg:demo/microservice/internal/handler", "file:demo/microservice/internal/handler/user.go", graph.EdgeTypeContains),
		e("pkg:demo/microservice/internal/service", "file:demo/microservice/internal/service/user_service.go", graph.EdgeTypeContains),
		e("pkg:demo/microservice/internal/repository", "file:demo/microservice/internal/repository/postgres.go", graph.EdgeTypeContains),

		// Function containment
		e("file:demo/microservice/internal/handler/user.go", "func:demo/microservice/internal/handler:HandleCreateUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/handler/user.go", "func:demo/microservice/internal/handler:HandleGetUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/handler/user.go", "func:demo/microservice/internal/handler:HandleListUsers", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/handler/user.go", "func:demo/microservice/internal/handler:HandleDeleteUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/service/user_service.go", "func:demo/microservice/internal/service:CreateUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/service/user_service.go", "func:demo/microservice/internal/service:ValidateUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/repository/postgres.go", "func:demo/microservice/internal/repository:InsertUser", graph.EdgeTypeContains),
		e("file:demo/microservice/internal/repository/postgres.go", "func:demo/microservice/internal/repository:FindUserByID", graph.EdgeTypeContains),

		// Call edges: handler → service → repository
		e("func:demo/microservice/internal/handler:HandleCreateUser", "func:demo/microservice/internal/service:CreateUser", graph.EdgeTypeCalls),
		e("func:demo/microservice/internal/handler:HandleGetUser", "func:demo/microservice/internal/repository:FindUserByID", graph.EdgeTypeCalls),
		e("func:demo/microservice/internal/service:CreateUser", "func:demo/microservice/internal/service:ValidateUser", graph.EdgeTypeCalls),
		e("func:demo/microservice/internal/service:CreateUser", "func:demo/microservice/internal/repository:InsertUser", graph.EdgeTypeCalls),

		// Implements edges
		e("struct:demo/microservice/internal/model:User", "interface:demo/microservice/internal/model:Validatable", graph.EdgeTypeImplements),

		// Cloud connections
		e("func:demo/microservice/internal/repository:InsertUser", "cloud:aws:rds", graph.EdgeTypeConnectsTo),
		e("func:demo/microservice/internal/repository:FindUserByID", "cloud:aws:rds", graph.EdgeTypeConnectsTo),
		e("func:demo/microservice/internal/service:CreateUser", "cloud:aws:sqs", graph.EdgeTypeConnectsTo),

		// Package imports
		e("pkg:demo/microservice/internal/handler", "pkg:demo/microservice/internal/service", graph.EdgeTypeImports),
		e("pkg:demo/microservice/internal/handler", "pkg:demo/microservice/internal/model", graph.EdgeTypeImports),
		e("pkg:demo/microservice/internal/service", "pkg:demo/microservice/internal/repository", graph.EdgeTypeImports),
		e("pkg:demo/microservice/internal/service", "pkg:demo/microservice/internal/model", graph.EdgeTypeImports),
	}
}
