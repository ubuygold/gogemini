import { useState } from 'react';

type LoginProps = {
  onLogin: (password: string) => void;
};

function Login({ onLogin }: LoginProps) {
  const [password, setPassword] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onLogin(password);
  };

  return (
    <div className="flex items-center justify-center h-screen">
      <form onSubmit={handleSubmit} className="p-8 bg-white rounded shadow-md">
        <h1 className="text-2xl font-bold text-gray-800 mb-4">Login</h1>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="input input-bordered w-full mb-4"
          placeholder="Password"
        />
        <button type="submit" className="btn btn-primary w-full">
          Login
        </button>
      </form>
    </div>
  );
}

export default Login;