import express from 'express';
import path from 'path';
import { createServer as createViteServer } from 'vite';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

interface Comment {
  id: number;
  content: string;
  parentId: number | null;
  createdAt: string;
}

// In-memory store for demo purposes
let comments: Comment[] = [
  { id: 1, content: "Welcome to EchoStream! This is a root comment.", parentId: null, createdAt: new Date().toISOString() },
  { id: 2, content: "Glad to be here. This is a reply.", parentId: 1, createdAt: new Date().toISOString() },
  { id: 3, content: "Me too! Another reply to the first one.", parentId: 1, createdAt: new Date().toISOString() },
  { id: 4, content: "Nested reply to the second comment.", parentId: 2, createdAt: new Date().toISOString() },
  { id: 5, content: "Another root comment to test pagination.", parentId: null, createdAt: new Date().toISOString() },
];

let nextId = 6;

async function startServer() {
  const app = express();
  const PORT = 3000;

  app.use(express.json());

  // API Routes (Matching Go Router)
  app.get('/comments', (req, res) => {
    const limit = parseInt(req.query.limit as string) || 5;
    const offset = parseInt(req.query.offset as string) || 0;
    const search = (req.query.search as string || '').toLowerCase();

    let filtered = comments.filter(c => c.parentId === null);

    if (search) {
      filtered = comments.filter(c => c.content.toLowerCase().includes(search));
    }

    const paginated = filtered.slice(offset, offset + limit);
    res.json(paginated);
  });

  // New endpoint from Go router: Get full tree
  app.get('/comments/all', (req, res) => {
    res.json(comments);
  });

  app.get('/comments/:parent_id/children', (req, res) => {
    const parentId = parseInt(req.params.parent_id);
    const children = comments.filter(c => c.parentId === parentId);
    res.json(children);
  });

  app.post('/comments', (req, res) => {
    const { content, parent_id } = req.body;
    if (!content) return res.status(400).json({ error: 'Content is required' });

    const newComment: Comment = {
      id: nextId++,
      content,
      parentId: parent_id || null,
      createdAt: new Date().toISOString()
    };

    comments.push(newComment);
    res.status(201).json({ 'Comment Created': newComment.id, comment: newComment });
  });

  app.delete('/comments/:id', (req, res) => {
    const id = parseInt(req.params.id);
    const index = comments.findIndex(c => c.id === id);
    if (index === -1) return res.status(404).json({ error: 'Comment not found' });

    comments.splice(index, 1);
    res.status(204).send();
  });

  // Vite middleware
  if (process.env.NODE_ENV !== 'production') {
    const vite = await createViteServer({
      server: { middlewareMode: true },
      appType: 'spa',
    });
    app.use(vite.middlewares);
  } else {
    const distPath = path.join(__dirname, 'dist');
    app.use(express.static(distPath));
    app.get('*', (req, res) => {
      res.sendFile(path.join(distPath, 'index.html'));
    });
  }

  app.listen(PORT, '0.0.0.0', () => {
    console.log(`Server running on http://localhost:${PORT}`);
  });
}

startServer();
