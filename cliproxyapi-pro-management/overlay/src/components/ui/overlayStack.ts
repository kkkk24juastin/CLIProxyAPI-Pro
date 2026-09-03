const openLayers: string[] = [];

export const registerOverlayLayer = (id: string) => {
  const existingIndex = openLayers.indexOf(id);
  if (existingIndex >= 0) openLayers.splice(existingIndex, 1);
  openLayers.push(id);

  return () => {
    const index = openLayers.indexOf(id);
    if (index >= 0) openLayers.splice(index, 1);
  };
};

export const isTopOverlayLayer = (id: string) =>
  openLayers[openLayers.length - 1] === id;
