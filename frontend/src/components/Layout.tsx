import { useEffect } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import '../styles/Layout.css'

export default function Layout() {
    const location = useLocation()
    const navigate = useNavigate()
    const { user, logout } = useAuth()

    // Report our height to an embedding parent (datapot.org tools page),
    // so it can size the iframe to fit without a nested scrollbar.
    // No-op when the app is viewed standalone.
    useEffect(() => {
        if (window.self === window.top) return
        const report = () =>
            window.parent.postMessage({ height: document.documentElement.scrollHeight }, '*')
        const observer = new ResizeObserver(report)
        observer.observe(document.body)
        report()
        return () => observer.disconnect()
    }, [])

    function handleLogout() {
        logout()
        navigate('/')
    }

    return (
        <div className="app-shell">
            <header className="masthead">
                <Link to="/" className="appname"><b>DMS</b> — Dataset Management System</Link>
                <nav className="app-nav">
                    <Link
                        to="/"
                        className={`app-nav-link ${location.pathname === '/' ? 'active' : ''}`}
                    >
                        Browse
                    </Link>
                    <Link
                        to="/upload"
                        className={`app-nav-link ${location.pathname === '/upload' ? 'active' : ''}`}
                    >
                        Upload
                    </Link>
                </nav>
                <nav className="app-nav-account">
                    {user ? (
                        <div className="app-user-menu">
                            <span className="app-user-name">{user.display_name}</span>
                            <button
                                type="button"
                                className="app-nav-button"
                                onClick={handleLogout}
                            >
                                Sign out
                            </button>
                        </div>
                    ) : (
                        <Link
                            to="/login"
                            className={`app-nav-link ${location.pathname === '/login' ? 'active' : ''}`}
                        >
                            Sign in
                        </Link>
                    )}
                </nav>
            </header>

            <main className="app-main">
                <Outlet />
            </main>
        </div>
    )
}
