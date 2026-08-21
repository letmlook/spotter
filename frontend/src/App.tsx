import TitleBar from './components/TitleBar';

export default function App() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ flex: 1, padding: 24 }}>Spotter</div>
    </div>
  );
}