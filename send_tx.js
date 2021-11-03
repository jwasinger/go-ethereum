const Web3 = require('web3')
const web3 = new Web3('http://localhost:8545')

/*
web3.eth.getAccounts(function(error, result) {
        console.log(web3.eth.accounts)
		web3.eth.sendTransaction(
			{
			from:"34a600a929c439fcc9fd87bf493fea453add3d5f",
			to:"34a600a929c439fcc9fd87bf493fea453add3d5a",
			value:  "1000000000000000000", 
			data: "0xdf"
				}, function(err, transactionHash) {
		  if (!err)
			console.log(transactionHash + " success"); 
		});
});
*/

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function sendTxs(tx, count) {
	let promises = []
	for (let i = 0; i < count; i++) {
		promises.push(new Promise((resolve, reject) => {web3.eth.sendTransaction(tx).then((x,y) => resolve())}))
	}
	return Promise.all(promises)
}

async function pollPending() {
	for (;;) {
		let pending = await new Promise((resolve, reject) => {
			web3.eth.getPendingTransactions().then((f) => resolve(f))})

		if (pending.length == 0) {
			console.log("no more pending")
			return
		} else {
			console.log("pending: ", pending.length)
		}
		await sleep(1000)
	}
}

async function main() {
	for (;;) {
		console.log("sending more txs...")
		await sendTxs({
			from:"34a600a929c439fcc9fd87bf493fea453add3d5f",
			to:"34a600a929c439fcc9fd87bf493fea453add3d50",
			value:  "1"//,
			//data: "0x60026000f3"
		}, 4096)
		await pollPending()
	}
}

main().then(() => {console.log("done")})
