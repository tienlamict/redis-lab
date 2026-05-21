import { Navigate, Route, Routes } from "react-router-dom";
import { Navbar } from "./components/Navbar";
import { StandalonePage } from "./pages/Standalone";
import { SentinelPage } from "./pages/Sentinel";
import { ClusterPage } from "./pages/Cluster";

export default function App() {
  return (
    <div className="flex h-screen flex-col bg-slate-900 text-slate-100">
      <Navbar />
      <main className="flex-1 min-h-0">
        <Routes>
          <Route path="/" element={<Navigate to="/standalone" replace />} />
          <Route path="/standalone" element={<StandalonePage />} />
          <Route path="/sentinel" element={<SentinelPage />} />
          <Route path="/cluster" element={<ClusterPage />} />
        </Routes>
      </main>
    </div>
  );
}
