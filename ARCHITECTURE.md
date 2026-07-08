Below is brief but not detailed overview of the architecture of the application.

It is a multi-tiered system architecture designed for a dataset management and search application. It follows a top-down flow, moving from the user interface through the processing logic to the data storage and external integrations.

1. Frontend Layer (React)
The entry point is a React-based web frontend. It handles the user interface (UI) for several core features:

Search Interface: For querying data.
Data Management: Uploading/downloading files and browsing datasets.
Result Presentation: Displaying search results complete with citations.

2. Application Layer (Go Backend)
The core logic resides in a Go backend, which acts as the orchestrator for the system. It is broken down into four specialized modules:

API Layer: Manages routing and authentication.
Search: Handles the logic for hybrid search (BM25 + vectors).
AI Layer: Manages prompts and tagging logic.
File Handler: Manages chunked I/O for efficient file processing.

3. Integration Layer (University AI API)
The backend communicates with a specialized University AI API. This layer is responsible for high-level AI tasks, including:

Generating summaries and pseudo-queries.
Handling classification (labels and classes).
Performing search reranking to improve result accuracy.

4. Persistence Layer (Storage)
The architecture utilizes three distinct storage solutions:

PostgreSQL: Relational database used for structured metadata and user information.
Search Indexes: A dedicated store for BM25 and vector embeddings to facilitate fast retrieval.
File Storage: A repository for raw datasets and images.

5. External Integration
At the bottom of the stack is an External Dataset API. According to the diagram, the specific access details for this layer are currently pending and are expected by the end of the week.