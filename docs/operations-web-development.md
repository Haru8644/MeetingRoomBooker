# Operations Web Development Guide

## Purpose

`MeetingRoomBooker.OperationsWeb` is a React + TypeScript operations UI for MeetingRoomBooker.

The existing Blazor WebAssembly app remains responsible for the main reservation experience. The operations UI is added separately so conflict tracking and operational review features can be developed without disrupting the existing booking UI.

## Technology Stack

* React
* TypeScript
* Vite
* CSS Modules are not used yet
* API integration is not connected yet

## Why This UI Is Separate

The existing Blazor WebAssembly app is already used for meeting room reservations.

The operations UI has a different responsibility:

* review unresolved reservation overlaps,
* review actual room collisions,
* show operational summary metrics,
* support future manual reporting and classification workflows.

Keeping it separate allows the team to add a new operations-focused interface while preserving the existing reservation flow.

## Why Vite

This UI is an internal operations SPA.

It does not currently need server-side rendering, static site generation, or SEO-focused routing. Vite is used because it provides a lightweight React + TypeScript development environment and a simple production build.

If the UI grows into a larger application with advanced routing, authentication flows, or public-facing pages, the frontend framework choice can be reviewed again.

## Project Location

```text
MeetingRoomBooker.OperationsWeb
```

## Install Dependencies

From the operations UI project directory:

```powershell
cd MeetingRoomBooker.OperationsWeb
npm install
```

## Start Local Development Server

```powershell
npm run dev
```

The development server usually starts at:

```text
http://localhost:5173/
```

## Production Build

```powershell
npm run build
```

The build output is generated under:

```text
MeetingRoomBooker.OperationsWeb/dist
```

The `dist` directory is a build artifact and must not be committed.

## Current Scope

The current operations UI is a static scaffold.

It includes:

* page shell,
* room conflict tracking title,
* placeholder summary cards,
* worker/API/deployment status notes,
* responsive CSS styling.

It does not yet include:

* API integration,
* authentication handling,
* conflict record list,
* summary API connection,
* manual conflict report form,
* edit or classification actions,
* production deployment configuration.

## Verification Checklist

Before opening a pull request that changes the operations UI, run:

```powershell
cd MeetingRoomBooker.OperationsWeb
npm run build
cd ..
dotnet build
dotnet test
```

Also check that generated files are not staged:

```powershell
git status --short --untracked-files=all
```

Do not commit:

* `node_modules`
* `dist`
* `.vite`
* `.vs`
* `bin`
* `obj`
* `*.csproj.user`

## Future Work

Planned follow-up work:

1. Add operations UI build to GitHub Actions.
2. Add API client code for room conflict records.
3. Connect summary cards to the summary endpoint.
4. Add conflict record list.
5. Add manual conflict report form.
6. Add detail and update screens for classification.
7. Document deployment options for the operations UI.
