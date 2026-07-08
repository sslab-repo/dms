import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import { AuthProvider } from './context/AuthContext'
import Home from './pages/Home'
import Login from './pages/Login'
import Upload from './pages/Upload'
import DatasetDetail from './pages/DatasetDetail'

export default function App() {
    return (
        <BrowserRouter>
            <AuthProvider>
                <Routes>
                    <Route element={<Layout />}>
                        <Route path="/" element={<Home />} />
                        <Route path="/login" element={<Login />} />
                        <Route path="/upload" element={<Upload />} />
                        <Route path="/datasets/:id" element={<DatasetDetail />} />
                    </Route>
                </Routes>
            </AuthProvider>
        </BrowserRouter>
    )
}
