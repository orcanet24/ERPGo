<style>
.btn-c {
	background-color: #10A37F;
	margin-top: 13px;
	transition: 0.8s;
}

.btn-c:hover {
	background-color: rgb(12, 124, 96);
}

.flex_center_btn {
	display: flex !important;
	justify-content: center !important;
	align-items: center !important;
	gap: 6px !important;
	padding: 10px 16px !important;
	border-radius: 6px !important;
}

.btn_ai_setting {
	background-color: black;
}
</style>

<a href="<?php echo base_url('home') ?>" class="logo">
	<?php
    if ($setting->logo) {
        $logo = $setting->logo ?? '';
    } else {
        $logo = 'assets/img/icons/logo.png';
    }

    if ($setting->favicon) {
        $favicon = $setting->favicon ?? '';
    } else {
        $favicon = 'assets/img/icons/favicon.png';
    }
    ?>
	<span class="logo-lg">
		<img src="<?php echo base_url($logo) ?>" alt="">
	</span>
	<span class="logo-mini">

		<img src="<?php echo base_url($favicon) ?>" alt="">
	</span>
</a>
<div class="se-pre-con"></div>
<!-- Header Navbar -->
<?php $gui_p = $this->uri->segment(1);
if ($gui_p != 'gui_pos') {
?>
<nav class="navbar navbar-static-top">
	<a href="#" class="sidebar-toggle" data-toggle="offcanvas" role="button">
		<!-- Sidebar toggle button-->
		<span class="sr-only">Toggle navigation</span>
		<span class="pe-7s-keypad"></span>
	</a>
	<span class="top-fixed-link">
		<?php

            if ($this->permission1->method('new_invoice', 'create')->access()) {
            ?>
		<a href="<?php echo base_url('add_invoice') ?>" class="btn btn-success btn-outline"><i
				class="fa fa-balance-scale"></i> <?php echo display('invoice') ?></a>
		<?php } ?>


		<?php if ($this->permission1->method('customer_receive', 'create')->access()) { ?>
		<a href="<?php echo base_url('customer_receive') ?>" class="btn btn-success btn-outline"><i class="fa fa-money"></i>
			<?php echo display('customer_receive') ?></a>
		<?php } ?>

		<?php if ($this->permission1->method('supplier_payment', 'create')->access()) { ?>
		<a href="<?php echo base_url('supplier_payment') ?>" class="btn btn-success btn-outline"><i class="fa fa-money"
				aria-hidden="true"></i> <?php echo display('supplier_payment') ?></a>
		<?php } ?>

		<?php if ($this->permission1->method('add_purchase', 'create')->access()) { ?>
		<a href="<?php echo base_url('add_purchase') ?>" class="btn btn-success btn-outline"><i