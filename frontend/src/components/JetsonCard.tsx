import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';

export default function JetsonCard({ device }: { device: RegistryEntry }) {
  const j = device.last_info?.jetson;
  if (!j) {
    return (
      <Card title="Jetson" size="small">
        <span style={{ color: '#888', fontStyle: 'italic' }}>Not a Jetson device or probe failed</span>
      </Card>
    );
  }
  const items = [
    { key: 'm', label: 'Model', v: j.model },
    { key: 'j', label: 'JetPack', v: j.jetpack },
    { key: 'l', label: 'L4T', v: j.l4t },
    { key: 'c', label: 'CUDA', v: j.cuda },
    { key: 'd', label: 'cuDNN', v: j.cudnn },
    { key: 't', label: 'TensorRT', v: j.tensorrt },
    { key: 'p', label: 'Python', v: j.python },
    { key: 's', label: 'Serial', v: j.serial },
  ].filter((it) => it.v);
  if (items.length === 0) {
    return (
      <Card title="Jetson" size="small">
        <span style={{ color: '#888', fontStyle: 'italic' }}>No Jetson probes succeeded</span>
      </Card>
    );
  }
  return (
    <Card title="Jetson" size="small">
      <Descriptions column={1} size="small" colon={false} labelStyle={{ color: '#888', width: 130 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.v}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}