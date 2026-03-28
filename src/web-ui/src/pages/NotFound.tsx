import React from 'react';

export const NotFound: React.FC = () => {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', flexDirection: 'column', gap: 16 }}>
      <h1>404</h1>
      <p>Page not found</p>
      <a href="/trade">Go to Trading</a>
    </div>
  );
};
