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

async function pollPending() {
	for (;;) {
		web3.eth.getPendingTransactions().then((f) => {console.log(f.length)})
		await new Promise(r => setTimeout(r, 20));
	}
}

for (let i = 0; i < 1; i++) {
		web3.eth.sendTransaction(
			{
			from:"34a600a929c439fcc9fd87bf493fea453add3d5f",
			to:"34a600a929c439fcc9fd87bf493fea453add3d50",
			value:  "1"//,
			//data: "0x60026000f3"
				})
}

pollPending()
