import { Routes, Route, Navigate } from 'react-router-dom';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import OBSPage from './pages/OBSPage';
import OBSSettings from './pages/OBSSettings';
import MultiStream from './pages/MultiStream';
import SettingsIngests from './pages/SettingsIngests';
import SettingsLogs from './pages/SettingsLogs';
import SettingsPassword from './pages/SettingsPassword';
import SettingsUpdate from './pages/SettingsUpdate';
import Recordings from './pages/Recordings';
import AutoSceneSwitcher from './pages/AutoSceneSwitcher';
import Layout from './components/Layout';

import RequireAuth from './components/RequireAuth';

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />

      {/* Protected Routes */}
      <Route element={<RequireAuth />}>
        <Route element={<Layout />}>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/obs" element={<OBSPage />} />
          <Route path="/obs/settings" element={<OBSSettings />} />
          <Route path="/obs/multistream" element={<MultiStream />} />
          <Route path="/recordings" element={<Recordings />} />

          <Route path="/settings/ingests" element={<SettingsIngests />} />
          <Route path="/settings/scene-switcher" element={<AutoSceneSwitcher />} />
          <Route path="/settings/logs" element={<SettingsLogs />} />
          <Route path="/settings/password" element={<SettingsPassword />} />
          <Route path="/settings/update" element={<SettingsUpdate />} />
        </Route>
      </Route>
    </Routes>
  );
}

export default App;

