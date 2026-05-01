import React from "react";
import { Outlet, NavLink } from "react-router-dom";

export default function Layout() {
  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <nav className="w-64 flex-shrink-0 overflow-y-auto p-4 bg-white border-r border-gray-200 text-gray-700">
        <h2 className="text-lg font-bold mb-6 px-4">Dashboard</h2>
      <div className="mb-6">
        <h3 className="px-4 text-xs font-semibold uppercase tracking-wider opacity-60 mb-2">Dashboard Api</h3>
        <NavLink to="/dashboard-api/dashboard" className={({ isActive }) => `block px-4 py-2 text-sm rounded-md ${isActive ? "bg-blue-50 text-blue-700 border-r-2 border-blue-600" : "hover:bg-black/5"}`}>Dashboard Screen</NavLink>
      </div>
      </nav>

      {/* Main content */}
      <div className="flex-1 flex flex-col overflow-hidden">
        <header className="h-14 flex items-center px-6 bg-white border-b border-gray-200 text-gray-900">
          <h1 className="text-sm font-medium">Dashboard</h1>
        </header>
        <main className="flex-1 overflow-y-auto p-6 bg-gray-50">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
