import { useState, useEffect } from 'react';
import Login from './components/Login';
import GeminiKeyManager from './components/GeminiKeyManager';
import ClientKeyManager from './components/ClientKeyManager';
import Navbar from './components/Navbar';

function App() {
	const [loggedIn, setLoggedIn] = useState(false);
	const [password, setPassword] = useState('');
	const [activeTab, setActiveTab] = useState<'gemini' | 'client'>('gemini');
	const [loading, setLoading] = useState(true); // Add loading state

	useEffect(() => {
		const checkLogin = async () => {
			const sessionData = sessionStorage.getItem('adminSession');
			if (sessionData) {
				const { password: storedPassword, timestamp } = JSON.parse(sessionData);
				const oneHour = 60 * 60 * 1000;

				if (Date.now() - timestamp > oneHour) {
					sessionStorage.removeItem('adminSession');
				} else {
					try {
						const response = await fetch('/admin/gemini-keys', {
							headers: {
								Authorization: `Basic ${btoa(`admin:${storedPassword}`)}`,
							},
						});
						if (response.ok) {
							setLoggedIn(true);
							setPassword(storedPassword);
						} else {
							sessionStorage.removeItem('adminSession');
						}
					} catch (error) {
						console.error('Error checking login status:', error);
						sessionStorage.removeItem('adminSession');
					}
				}
			}
			setLoading(false);
		};

		checkLogin();
	}, []);

	const handleLogin = async (password: string) => {
		try {
			const response = await fetch('/admin/gemini-keys', {
				headers: {
					Authorization: `Basic ${btoa(`admin:${password}`)}`,
				},
			});
			if (response.ok) {
				setLoggedIn(true);
				setPassword(password);
				const sessionData = { password: password, timestamp: Date.now() };
				sessionStorage.setItem('adminSession', JSON.stringify(sessionData));
			} else {
				alert('Login failed');
			}
		} catch (error) {
			console.error('Login error:', error);
			alert('Login failed');
		}
	};

	const handleLogout = () => {
		setLoggedIn(false);
		setPassword('');
		sessionStorage.removeItem('adminSession');
	};

	if (loading) {
		return <div>Loading...</div>; // Or a spinner component
	}

	if (!loggedIn) {
		return <Login onLogin={handleLogin} />;
	}

	return (
		<div>
			<Navbar
				activeTab={activeTab}
				onTabChange={setActiveTab}
				onLogout={handleLogout}
			/>
			<div className="container mx-auto p-4">
				<div className="mt-4">
					{activeTab === 'gemini' && <GeminiKeyManager password={password} />}
					{activeTab === 'client' && <ClientKeyManager password={password} />}
				</div>
			</div>
		</div>
	);
}

export default App;
