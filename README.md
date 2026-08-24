# Sandbox API Service

## The Problem

Every E2B sandbox is created, monitored, and killed through our public API.
Developers authenticate with Bearer tokens, and behind every request is a user, a
project, and a plan with limits. When we get this layer wrong, one runaway agent
loop can hammer the platform or developers can't tell why their requests are
failing.

**Your job**: Build a small version of E2B's public API service — the auth, projects,
and rate-limiting/quota system that sits in front of the sandboxes.

**The domain model**:
  - **Users** sign in with OAuth and get a JWT
  - **Projects** are the tenancy unit — a user can have many projects, and a project
    can be shared by many users
  - **Sandboxes** always run inside a project

## Getting Started

At the end of this document you are provided `docker compose up` with **PostgreSQL**,
**Redis**, and **Ory Hydra** — a pre-configured OAuth2 provider with a demo login
page and a test user, so you don't need any external accounts

**Before building**:
  - Bring the compose stack up and log in through the demo Hydra login page
  - Find the pre-configured OAuth client credentials and redirect URI in the
    compose file

## What to Build

Build a Go service that exposes:
  - user signs up with OAuth flow
  - user interacts with the platform using OAuth JWTs
  - users can create a project; list projects - add another user to a project - after
    that, it's theirs too
  - only members can touch a project and its sandboxes
  - `POST /v1/projects/{id}/sandboxes` — creates a sandbox record (fake lifecycle is
  fine: running , stopped )
  - `GET /v1/projects/{id}/sandboxes` — lists the project's sandboxes
  - `DELETE /v1/sandboxes/{id}` — stops one
  - all Postgres access goes through sqlc-generated code
  - schema migrations included — users, projects, sandboxes, etc ...
  
From an architectural perspective, we'll evaluate the design decisions throughout
your submission. Ask yourself: Would this architecture support millions of users
and billions of sandboxes?

We don't expect the implementation itself to scale to that level, but the underlying
architecture should.

If you have ideas for improvements that you didn't have time to implement, feel
free to describe them in a short Markdown document outlining your proposed
solution.