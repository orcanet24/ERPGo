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
				class="ti-shopping-cart"></i> <?php echo display('purchase') ?></a>
		<?php } ?>
	</span>

	<div class="navbar-custom-menu">

		<ul class="nav navbar-nav">
			<!-- Messages -->
			<?php if ($this->permission1->method('pos_invoice', 'create')->access()) {
                ?>
			<!-- <li style="margin-right: 6px;">
				<a href="<?php echo base_url('ai_settings') ?>" class="text-white btn-c pos-btn flex_center_btn btn_ai_setting">
					<span> <i style="font-size: 16px;" class="ti-settings"></i></span> <?php echo display('ai_setting') ?></a>
			</li> -->
			<li style="margin-right: 6px; font-size:19px;">
				<a href="<?php echo base_url('open_ai') ?>" class="text-white  btn-c pos-btn flex_center_btn"> <span>
						<!-- <svg width="16" height="16" viewBox="0 0 25 26" fill="none" xmlns="http://www.w3.org/2000/svg">
							<path
								d="M23.3579 10.6395C23.6412 9.764 23.7392 8.83658 23.6453 7.91916C23.5514 7.00173 23.2677 6.11543 22.8133 5.31945C22.1395 4.11633 21.1107 3.16379 19.8752 2.59916C18.6397 2.03453 17.2613 1.88698 15.9388 2.17779C15.1875 1.3204 14.2294 0.680923 13.1609 0.323577C12.0924 -0.0337692 10.9511 -0.0964001 9.8515 0.141975C8.75194 0.38035 7.73287 0.911338 6.89665 1.68161C6.06043 2.45188 5.4365 3.43432 5.08752 4.53024C4.20645 4.71559 3.37409 5.09172 2.64607 5.6335C1.91805 6.17528 1.31113 6.87023 0.865888 7.6719C0.184795 8.87301 -0.1063 10.2652 0.0346883 11.6472C0.175677 13.0292 0.741442 14.3294 1.65019 15.3599C1.36577 16.2349 1.26678 17.1622 1.35984 18.0796C1.4529 18.9971 1.73587 19.8836 2.18981 20.6798C2.86444 21.8833 3.89414 22.8361 5.13051 23.4007C6.36688 23.9653 7.74608 24.1127 9.06933 23.8215C9.66626 24.5111 10.3997 25.0622 11.2208 25.4378C12.0418 25.8134 12.9315 26.0051 13.8305 25.9999C15.186 26.0012 16.5069 25.5605 17.6024 24.7416C18.698 23.9226 19.5115 22.7678 19.9256 21.4436C20.8066 21.258 21.6388 20.8817 22.3668 20.3399C23.0948 19.7982 23.7017 19.1033 24.1472 18.3019C24.8203 17.1025 25.1064 15.7155 24.9646 14.3393C24.8228 12.9631 24.2604 11.668 23.3579 10.6395ZM13.8305 24.2982C12.7203 24.3 11.645 23.9008 10.793 23.1707L10.9428 23.0836L15.9888 20.0955C16.1144 20.02 16.2186 19.9122 16.2913 19.7828C16.364 19.6534 16.4027 19.5068 16.4035 19.3575V12.0592L18.5366 13.3252C18.5472 13.3306 18.5563 13.3386 18.5633 13.3484C18.5702 13.3582 18.5749 13.3695 18.5767 13.3815V19.4293C18.5741 20.7198 18.0732 21.9566 17.1836 22.8691C16.2941 23.7816 15.0884 24.2955 13.8305 24.2982ZM3.62864 19.829C3.07187 18.8428 2.87197 17.6867 3.06409 16.5642L3.21404 16.6565L8.26497 19.6445C8.38994 19.7198 8.53222 19.7594 8.67713 19.7594C8.82203 19.7594 8.96431 19.7198 9.08929 19.6445L15.2594 15.9953V18.5221C15.2588 18.5352 15.2553 18.548 15.2493 18.5595C15.2432 18.5711 15.2347 18.581 15.2243 18.5887L10.1134 21.6126C9.02269 22.2572 7.72719 22.4314 6.51136 22.097C5.29553 21.7626 4.25874 20.9469 3.... [truncated]
								fill="white" />
						</svg> -->
					</span> <?php echo display('open_ai') ?></a>
			</li>
			<li>
				<a href="<?php echo base_url('gui_pos') ?>" class="text-white  btn-success pos-btn flex_center_btn"> <span
						class="fa fa-plus"></span> <?php echo display('pos_invoice') ?></a>
			</li>
			<?php } ?>
			<?php if ($max_version > $current_version) { ?>
			<li>
				<blink><a href="<?php echo base_url('autoupdate/Autoupdate') ?>" class="text-white  btn-danger update-btn">
						<?php echo $max_version . ' Version Available'; ?></a>
				</blink>
			</li>
			<?php } ?>
			<li class="dropdown notifications-menu">
				<a href="<?php echo base_url('out_of_stock') ?>">
					<i class="pe-7s-attention" title="<?php echo display('out_of_stock') ?>"></i>
					<span class="label label-danger"><?php echo html_escape($out_of_stocks) ?></span>
				</a>
			</li>
			<!-- settings -->
			<li class="dropdown dropdown-user">
				<a href="#" class="dropdown-toggle" data-toggle="dropdown"> <i class="pe-7s-settings"></i></a>
				<ul class="dropdown-menu">
					<li><a href="<?php echo base_url('edit_profile') ?>"><i class="pe-7s-users"></i>
							<?php echo display('edit_profile') ?></a></li>
					<li><a href="<?php echo base_url('change_password') ?>"><i
								class="pe-7s-settings"></i><?php echo display('change_password') ?></a></li>
					<li><a href="<?php echo base_url('logout') ?>"><i class="pe-7s-key"></i>
							<?php echo display('logout') ?></a></li>
				</ul>
			</li>
		</ul>
	</div>

</nav>
<?php } ?>