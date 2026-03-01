# 🏢 MeetingRoomBooker

![.NET](https://img.shields.io/badge/.NET-8.0-512BD4?style=for-the-badge&logo=dotnet&logoColor=white)
![Blazor](https://img.shields.io/badge/Blazor-WASM-5C2D91?style=for-the-badge&logo=blazor&logoColor=white)
![C#](https://img.shields.io/badge/c%23-%23239120.svg?style=for-the-badge&logo=csharp&logoColor=white)
![SQLite](https://img.shields.io/badge/sqlite-%2307405e.svg?style=for-the-badge&logo=sqlite&logoColor=white)
![Nginx](https://img.shields.io/badge/nginx-%23009639.svg?style=for-the-badge&logo=nginx&logoColor=white)
![Linux](https://img.shields.io/badge/Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)

> **A Modern SPA designed to eliminate double-bookings, streamline enterprise resource management, and drive true digital transformation (DX) on the front lines.**

## 📖 Overview
MeetingRoomBooker is a comprehensive conference room reservation platform developed from scratch to solve real-world operational bottlenecks. Built entirely on the Microsoft C#/.NET ecosystem, it features a highly decoupled architecture utilizing **Blazor WebAssembly** for a rich, responsive frontend and **ASP.NET Core Web API** for a robust, secure backend. 

The UI features a modern, cyberpunk-inspired Glassmorphism design integrated with **Microsoft Fluent UI**, delivering an intuitive user experience that employees genuinely *want* to use.

## 🚀 The Challenge & Business Impact
**The Problem:** Previously, reservations were managed via generic calendar apps (e.g., TimeTree). Due to poor visibility and lack of conflict-prevention mechanisms, **double-bookings occurred 5-6 times a month**, causing a constant monthly loss of 1-2 hours in coordination and delaying critical business decisions.

**The Solution & Results:**
By redefining the root causes (mixed use cases, undefined exception handling, lack of clear responsibility) and implementing this dedicated system:
- 📉 **Double-bookings reduced to exactly ZERO.**
- ⏱️ **Coordination time slashed by 100%**, reclaiming hours of lost productivity.
- 🤝 **Seamless Adoption:** Established a "5-Step Framework" (Exception Categorization, Responsibility Demarcation, Authority/Audit, Migration Procedure, Adoption Metrics) to ensure the system didn't just end at "deployment" but achieved full-scale company adoption.

## ✨ Key Features
- **Real-Time Conflict Prevention:** Backend validation ensures overlapping reservations are mathematically impossible.
- **Modern UI/UX:** Cyberpunk aesthetics with Glassmorphism, powered by CSS animations and Fluent UI components for enterprise-grade accessibility.
- **Color-Coded Timeline:** Instantly distinguish between "Internal Meetings" (Purple) and "Client Visits" (Green) to maintain professionalism on the floor.
- **RESTful API Architecture:** Strict adherence to REST principles (GET, POST, PUT, DELETE) using JSON payloads.
- **End-to-End Type Safety:** Utilizing C# shared models across both frontend (WASM) and backend (API) eliminates data-mismatch bugs during compilation.

## 🏗️ Architecture & Infrastructure
The application is currently deployed and maintained on a Linux VPS, demonstrating full-stack deployment and infrastructure management capabilities.

- **Frontend:** Blazor WebAssembly (SPA)
- **Backend:** ASP.NET Core Web API, Entity Framework Core (EF Core)
- **Database:** SQLite
- **Web Server / Reverse Proxy:** Nginx handling internet traffic, routing to internal Kestrel servers.
- **Process Management:** `systemd` daemonizing the API for 24/7 high availability and auto-healing.

## 💡 Why the Microsoft .NET Ecosystem?
In enterprise environments, technology must balance agility with governance. I chose the .NET stack because it provides a unified, strongly-typed ecosystem. Sharing C# models between the client and server drastically speeds up development and reduces human error. Furthermore, deploying ASP.NET Core on Linux via Kestrel proves the cross-platform power of modern .NET, ensuring long-term maintainability (LTS) and enterprise-grade security.

## 🛠️ Getting Started (Local Development)

### Prerequisites
- [.NET 8.0 SDK](https://dotnet.microsoft.com/download/dotnet/8.0)
- Visual Studio 2022 / VS Code

### 1. Run the API (Backend)
Open a terminal and run the following commands:

```bash
cd MeetingRoomBooker.Api
dotnet run
```

### 2. Run the Web App (Frontend)
Open a new terminal window and run:

```bash
cd MeetingRoomBooker.Web
dotnet run
```

*The web app will automatically connect to the local API endpoint.*
