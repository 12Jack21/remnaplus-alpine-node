import { writeFile } from 'node:fs/promises';

import { encodeCertPayload } from '../../backend/src/common/utils/certs/encode-node-payload';
import {
    generateJwtKeypair,
    generateMasterCerts,
    generateNodeCert,
} from '../../backend/src/common/utils/certs/generate-certs.util';

async function main(): Promise<void> {
    const output = process.argv[2];
    if (!output) throw new Error('Provide a new disposable fixture path');
    const master = await generateMasterCerts();
    const node = await generateNodeCert(master.caCertPem, master.caKeyPem);
    const jwt = await generateJwtKeypair();
    const payload = encodeCertPayload({
        caCertPem: master.caCertPem,
        jwtPublicKey: jwt.publicKey,
        nodeCertPem: node.nodeCertPem,
        nodeKeyPem: node.nodeKeyPem,
    });
    await writeFile(output, payload, { flag: 'wx', mode: 0o600 });
}

void main().catch(() => {
    console.error('Disposable dashboard secret fixture generation failed');
    process.exitCode = 1;
});
