# EchoStream - React Frontend

This is the React frontend for the CommentTree service.

## How to run

### 1. Development with Mock Backend (Express)
If you want to run the frontend with the built-in Express mock server:
```bash
npm run dev
```
This will start the server on `http://localhost:3000`.

### 2. Development with Go Backend (Proxy)
If you want to connect to your Go backend running on `http://localhost:8080`:
1. Start your Go server.
2. Run the following command in this folder:
   ```bash
   npm run dev:vite
   ```
Vite will proxy all requests starting with `/comments` to your Go server.

### 3. Production Build
To build the static files for production:
```bash
npm run build
```
The output will be in the `dist` folder.

## Configuration
- **Proxy**: Configured in `vite.config.ts`.
- **Styling**: Tailwind CSS 4.0.
- **Icons**: Lucide React.
- **Animations**: Motion (framer-motion).
